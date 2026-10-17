//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

const (
	watcherName      = "org.kde.StatusNotifierWatcher"
	watcherPath      = dbus.ObjectPath("/StatusNotifierWatcher")
	watcherInterface = "org.kde.StatusNotifierWatcher"
)

// item is a StatusNotifierItem as the watcher recorded it.
type item struct {
	bus  string
	path dbus.ObjectPath
}

// watcher is a minimal StatusNotifierWatcher. A desktop panel provides the
// real one; the tray only needs a watcher to register with. Nothing draws the
// icon, because the test reads it over D-Bus.
type watcher struct {
	props *prop.Properties

	mu         sync.Mutex
	items      []item
	registered chan item
}

// RegisterStatusNotifierItem is the watcher method an item calls on startup.
func (w *watcher) RegisterStatusNotifierItem(sender dbus.Sender, service string) *dbus.Error {
	// The spec lets an item pass only its object path, with the bus name
	// implied by the sender. fyne.io/systray does this.
	it := item{bus: service, path: "/StatusNotifierItem"}
	if strings.HasPrefix(service, "/") {
		it = item{bus: string(sender), path: dbus.ObjectPath(service)}
	}

	w.mu.Lock()
	w.items = append(w.items, it)
	names := make([]string, 0, len(w.items))
	for _, registered := range w.items {
		names = append(names, registered.bus+string(registered.path))
	}
	w.mu.Unlock()

	w.props.SetMust(watcherInterface, "RegisteredStatusNotifierItems", names)
	select {
	case w.registered <- it:
	default: // the test reads only the first registration
	}
	return nil
}

// RegisterStatusNotifierHost accepts a host. The stub reports a host already.
func (w *watcher) RegisterStatusNotifierHost(string) *dbus.Error { return nil }

// startWatcher exports the watcher on conn and owns its well-known name.
func startWatcher(conn *dbus.Conn) (*watcher, error) {
	w := &watcher{registered: make(chan item, 1)}
	if err := conn.Export(w, watcherPath, watcherInterface); err != nil {
		return nil, fmt.Errorf("export watcher: %w", err)
	}

	props, err := prop.Export(conn, watcherPath, prop.Map{
		watcherInterface: {
			"RegisteredStatusNotifierItems": {Value: []string{}, Emit: prop.EmitTrue},
			// Some tray libraries register only when a host is reported.
			"IsStatusNotifierHostRegistered": {Value: true, Emit: prop.EmitTrue},
			"ProtocolVersion":                {Value: int32(0), Emit: prop.EmitTrue},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("export watcher properties: %w", err)
	}
	w.props = props

	node := &introspect.Node{
		Name: string(watcherPath),
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:       watcherInterface,
				Methods:    introspect.Methods(w),
				Properties: props.Introspection(watcherInterface),
			},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), watcherPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, fmt.Errorf("export watcher introspection: %w", err)
	}

	// A desktop always has a host on the bus, so the stub owns a host name too.
	for _, name := range []string{watcherName, fmt.Sprintf("org.kde.StatusNotifierHost-%d", os.Getpid())} {
		reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
		if err != nil {
			return nil, fmt.Errorf("request %s: %w", name, err)
		}
		if reply != dbus.RequestNameReplyPrimaryOwner {
			return nil, fmt.Errorf("request %s: not the primary owner (reply %d)", name, reply)
		}
	}
	return w, nil
}
