//go:build e2e

// Package e2e runs the tray binary against a real Ollama server and reads it
// over D-Bus the way a desktop panel does. Run it with `go run -tags task ./tasks/e2e`; it needs
// the session bus and environment that the e2e container sets up.
package e2e

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
)

const (
	grey  = "#8b949e"
	green = "#2ea043"
	red   = "#d73a49"

	// startupTimeout covers the tray's first D-Bus calls on a slow runner.
	startupTimeout = 30 * time.Second
	// updateTimeout is ten tray polls at the minimum one-second interval.
	updateTimeout = 10 * time.Second
	// unloadTimeout sits between a correct unload and a broken one. A request
	// with keep_alive 0 unloaded the model in tens of milliseconds here.
	// Ollama reads keep_alive 1 as one second, so that model leaves too late.
	unloadTimeout = 500 * time.Millisecond
)

func TestTrayFollowsARealServer(t *testing.T) {
	model := requireEnv(t, "E2E_MODEL")
	ollamaURL := requireEnv(t, "OLLAMA_URL")
	trayBinary := requireEnv(t, "OLLAMA_TRAY")
	ctx := t.Context()

	server := ollamaServer{baseURL: ollamaURL, http: &http.Client{}}
	require.NoError(t, server.pull(ctx, model))
	// A model left loaded by an earlier run would make the idle checks meaningless.
	loaded, err := server.loadedNames(ctx)
	require.NoError(t, err)
	for _, name := range loaded {
		require.NoError(t, server.unload(ctx, name))
	}

	target, err := url.Parse(ollamaURL)
	require.NoError(t, err)
	tcpProxy, err := startProxy(target.Host)
	require.NoError(t, err)
	t.Cleanup(tcpProxy.close)

	conn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	stub, err := startWatcher(conn)
	require.NoError(t, err)

	host := tcpProxy.url()
	startTray(t, trayBinary, host)
	var registered item
	select {
	case registered = <-stub.registered:
	case <-time.After(startupTimeout):
		t.Fatal("the tray did not register a StatusNotifierItem")
	}
	tray := newTrayItem(conn, registered)

	tip := waitUntil(t, "the idle tooltip", startupTimeout, tray.toolTip, func(tip toolTip) bool {
		return tip.Title == host+"\nno model loaded"
	})
	// fyne.io/systray puts the whole tooltip in the title and sends no text or icon.
	t.Logf("ToolTip as the tray sends it: %#v", tip)
	assertIconColor(t, tray, grey)

	require.NoError(t, server.load(ctx, model, "10m"))
	// The tray rounds the time left down, so a poll a moment after the load shows 9m.
	tip = waitUntil(t, "the time left of a 10 minute keep-alive", updateTimeout, tray.toolTip, func(tip toolTip) bool {
		return strings.Contains(tip.Title, "unloads in 9m)") || strings.Contains(tip.Title, "unloads in 10m)")
	})
	assert.True(t, strings.HasPrefix(tip.Title, host+"\n"+model+" ("), tip.Title)
	assertIconColor(t, tray, green)

	keepLabel := model + " > " + menu.KeepLoadedLabel
	items := waitUntil(t, "a visible Keep loaded item", updateTimeout, tray.visibleItems, func(paths map[string]int32) bool {
		_, ok := paths[keepLabel]
		return ok
	})
	require.NoError(t, tray.click(items[keepLabel]))
	waitUntil(t, "kept loaded after the click", updateTimeout, tray.toolTip, func(tip toolTip) bool {
		return strings.Contains(tip.Title, ", kept loaded)")
	})
	items = waitUntil(t, "Keep loaded to leave the menu", updateTimeout, tray.visibleItems, func(paths map[string]int32) bool {
		_, ok := paths[keepLabel]
		return !ok
	})

	stopLabel := model + " > " + menu.StopLabel
	_, ok := items[stopLabel]
	require.True(t, ok, "Stop must stay in the menu for a kept model: %v", items)
	clicked := time.Now()
	require.NoError(t, tray.click(items[stopLabel]))
	waitUntil(t, "Ollama to unload the model", unloadTimeout, func() ([]string, error) {
		return server.loadedNames(ctx)
	}, func(names []string) bool { return !slices.Contains(names, model) })
	t.Logf("Ollama unloaded the model %s after the click", time.Since(clicked))

	waitUntil(t, "the tooltip to show nothing loaded", updateTimeout, tray.toolTip, func(tip toolTip) bool {
		return tip.Title == host+"\nno model loaded"
	})
	assertIconColor(t, tray, grey)

	tcpProxy.close()
	waitUntil(t, "the tooltip to show the server unreachable", updateTimeout, tray.toolTip, func(tip toolTip) bool {
		return tip.Title == host+"\nunreachable"
	})
	assertIconColor(t, tray, red)
}

func requireEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is not set; run the test with go run -tags task ./tasks/e2e", name)
	}
	return value
}

// startTray runs the tray binary until the test ends. It polls every second,
// the minimum, so each wait above lasts about one poll.
func startTray(t *testing.T, binary, host string) {
	t.Helper()
	// The binary path comes from the e2e container's environment.
	cmd := exec.Command(binary, "--host", host, "--interval", "1s") //nolint:gosec
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		// SIGTERM is the tray's normal shutdown, so it releases its bus name.
		_ = cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	})
}

// assertIconColor reads the icon once. The tray sets the icon before the
// tooltip in each poll, so the icon is current once the tooltip is.
func assertIconColor(t *testing.T, tray *trayItem, want string) {
	t.Helper()
	got, err := tray.iconColor()
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
