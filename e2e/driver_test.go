//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	itemInterface = "org.kde.StatusNotifierItem"
	menuInterface = "com.canonical.dbusmenu"
	pollEvery     = 20 * time.Millisecond
)

// waitUntil calls read until accept passes, and fails the test with the last
// value or error after timeout. The clock is read before read runs, so a
// value read after the deadline never passes. The unload bound relies on
// this to reject a model that leaves late.
func waitUntil[T any](t *testing.T, what string, timeout time.Duration, read func() (T, error), accept func(T) bool) T {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		expired := time.Now().After(deadline)
		value, err := read()
		if err == nil && accept(value) {
			return value
		}
		if expired {
			t.Fatalf("waited %s for %s; last value %#v, last error %v", timeout, what, value, err)
		}
		time.Sleep(pollEvery)
	}
}

// pixmap is one ARGB32 image in a StatusNotifierItem property.
type pixmap struct {
	Width  int32
	Height int32
	ARGB   []byte
}

// toolTip is the StatusNotifierItem ToolTip property, signature (sa(iiay)ss).
type toolTip struct {
	IconName string
	Icons    []pixmap
	Title    string
	Text     string
}

// trayItem reads the tray's StatusNotifierItem and dbusmenu the way a panel does.
type trayItem struct {
	item dbus.BusObject
	conn *dbus.Conn
	bus  string
}

func newTrayItem(conn *dbus.Conn, registered item) *trayItem {
	return &trayItem{conn: conn, bus: registered.bus, item: conn.Object(registered.bus, registered.path)}
}

func (ti *trayItem) toolTip() (toolTip, error) {
	var tip toolTip
	v, err := ti.item.GetProperty(itemInterface + ".ToolTip")
	if err != nil {
		return tip, fmt.Errorf("get ToolTip: %w", err)
	}
	if err := v.Store(&tip); err != nil {
		return tip, fmt.Errorf("decode ToolTip %s: %w", v, err)
	}
	return tip, nil
}

// iconColor returns the center pixel of the largest icon as #rrggbb.
func (ti *trayItem) iconColor() (string, error) {
	v, err := ti.item.GetProperty(itemInterface + ".IconPixmap")
	if err != nil {
		return "", fmt.Errorf("get IconPixmap: %w", err)
	}
	var pixmaps []pixmap
	if err := v.Store(&pixmaps); err != nil {
		return "", fmt.Errorf("decode IconPixmap: %w", err)
	}
	if len(pixmaps) == 0 {
		return "", errors.New("no IconPixmap")
	}
	largest := pixmaps[0]
	for _, p := range pixmaps[1:] {
		if p.Width > largest.Width {
			largest = p
		}
	}
	// Each pixel is ARGB in network byte order, so the color is bytes 1 to 3.
	offset := int((largest.Height/2)*largest.Width+largest.Width/2) * 4
	if offset+4 > len(largest.ARGB) {
		return "", fmt.Errorf("IconPixmap %dx%d has only %d bytes", largest.Width, largest.Height, len(largest.ARGB))
	}
	return fmt.Sprintf("#%x", largest.ARGB[offset+1:offset+4]), nil
}

func (ti *trayItem) menu() (dbus.BusObject, error) {
	v, err := ti.item.GetProperty(itemInterface + ".Menu")
	if err != nil {
		return nil, fmt.Errorf("get Menu: %w", err)
	}
	path, ok := v.Value().(dbus.ObjectPath)
	if !ok {
		return nil, fmt.Errorf("menu path is %T", v.Value())
	}
	return ti.conn.Object(ti.bus, path), nil
}

// menuNode is one dbusmenu layout node, signature (ia{sv}av).
type menuNode struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// visibleItems maps the label of each visible menu item to its id.
func (ti *trayItem) visibleItems() (map[string]int32, error) {
	menu, err := ti.menu()
	if err != nil {
		return nil, err
	}
	var (
		revision uint32
		root     menuNode
	)
	if err := menu.Call(menuInterface+".GetLayout", 0, int32(0), int32(-1), []string{}).Store(&revision, &root); err != nil {
		return nil, fmt.Errorf("GetLayout: %w", err)
	}
	labels := map[string]int32{}
	for _, child := range root.Children {
		var node menuNode
		if err := child.Store(&node); err != nil {
			return nil, fmt.Errorf("decode menu item %s: %w", child, err)
		}
		// dbusmenu treats a missing "visible" as true.
		if visible, ok := node.Props["visible"].Value().(bool); ok && !visible {
			continue
		}
		if label, ok := node.Props["label"].Value().(string); ok {
			labels[label] = node.ID
		}
	}
	return labels, nil
}

func (ti *trayItem) click(id int32) error {
	menu, err := ti.menu()
	if err != nil {
		return err
	}
	if err := menu.Call(menuInterface+".Event", 0, id, "clicked", dbus.MakeVariant(int32(0)), uint32(0)).Err; err != nil {
		return fmt.Errorf("click item %d: %w", id, err)
	}
	return nil
}

// ollamaServer talks to Ollama directly, not through the proxy the tray uses.
type ollamaServer struct {
	baseURL string
	http    *http.Client
}

func (o ollamaServer) post(ctx context.Context, path string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s: %w", path, err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, reply)
	}
	return nil
}

func (o ollamaServer) pull(ctx context.Context, model string) error {
	return o.post(ctx, "/api/pull", map[string]any{"model": model, "stream": false})
}

// load loads model. A generate call with no prompt only loads it.
func (o ollamaServer) load(ctx context.Context, model string) error {
	return o.post(ctx, "/api/generate", map[string]any{"model": model, "keep_alive": "5m"})
}

func (o ollamaServer) unload(ctx context.Context, model string) error {
	return o.post(ctx, "/api/generate", map[string]any{"model": model, "keep_alive": 0})
}

func (o ollamaServer) loadedNames(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, fmt.Errorf("build /api/ps: %w", err)
	}
	resp, err := o.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/ps: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var ps struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, fmt.Errorf("decode /api/ps: %w", err)
	}
	names := make([]string, 0, len(ps.Models))
	for _, m := range ps.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// proxy forwards TCP connections from a loopback port to Ollama. The tray
// talks to Ollama only through it, so closing it makes the server
// unreachable without stopping the Ollama container. That would need the
// Docker socket, which gives a container control of the host's Docker.
type proxy struct {
	listener net.Listener
	target   string
	wg       sync.WaitGroup

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
}

func startProxy(target string) (*proxy, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}
	p := &proxy{listener: listener, target: target, conns: map[net.Conn]struct{}{}}
	p.wg.Go(p.accept)
	return p, nil
}

func (p *proxy) url() string { return "http://" + p.listener.Addr().String() }

func (p *proxy) accept() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return // the listener is closed
		}
		p.wg.Go(func() { p.forward(client) })
	}
}

func (p *proxy) forward(client net.Conn) {
	server, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	if !p.track(client, server) {
		return
	}
	var copies sync.WaitGroup
	copies.Go(func() {
		_, _ = io.Copy(server, client)
		_ = server.Close()
	})
	_, _ = io.Copy(client, server)
	_ = client.Close()
	copies.Wait()
	p.mu.Lock()
	delete(p.conns, client)
	delete(p.conns, server)
	p.mu.Unlock()
}

// track records a connection pair, or closes it when the proxy is already closed.
func (p *proxy) track(conns ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		for _, c := range conns {
			_ = c.Close()
		}
		return false
	}
	for _, c := range conns {
		p.conns[c] = struct{}{}
	}
	return true
}

// close stops accepting and drops every open connection, so the tray's
// kept-alive connections fail too.
func (p *proxy) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	_ = p.listener.Close()
	for c := range p.conns {
		_ = c.Close()
	}
	p.mu.Unlock()
	p.wg.Wait()
}
