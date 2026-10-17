// Package tray is the thin systray adapter. The decisions about what to show
// live in internal/status, internal/menu and internal/icon.
package tray

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

// Server is the part of the Ollama client the tray uses.
type Server interface {
	LoadedModels(ctx context.Context) ([]ollama.LoadedModel, error)
	Unload(ctx context.Context, name string) error
	KeepLoaded(ctx context.Context, name string) error
}

// Config is what Run needs from the command line.
type Config struct {
	Client   Server
	Host     string
	Interval time.Duration
}

// ui is every call the tray makes on the system tray, so tests can swap in a
// fake. fyne.io/systray cannot remove menu items, so the model items form a
// fixed pool addressed by slot.
type ui interface {
	SetIcon(s status.Status)
	SetTooltip(text string)
	ShowModel(slot int, title string)
	HideModel(slot int)
	ShowKeepLoaded(slot int, title string)
	HideKeepLoaded(slot int)
	SetKeepLoadedVisible(visible bool)
	SetStopAllVisible(visible bool)
}

const (
	// maxConcurrentUnloads bounds the unloads "Stop all" has in flight.
	maxConcurrentUnloads = menu.PoolSize
	// actionQueueSize is how many requests can wait behind a running one.
	actionQueueSize = 16
)

type actionKind int

const (
	actionStop actionKind = iota
	actionKeepLoaded
)

// action is one queued menu request for the named models.
type action struct {
	kind  actionKind
	names []string
}

// applied is what the last poll put on screen.
type applied struct {
	status  status.Status
	tooltip string
	layout  menu.Layout
}

type app struct {
	cfg     Config
	ui      ui
	refresh chan struct{}
	actions chan action
	// now is the clock; tests replace it to expire an action failure without waiting.
	now func() time.Time

	// last is read and written only by the poll goroutine.
	last    applied
	hasLast bool

	mu      sync.Mutex
	models  []ollama.LoadedModel
	layout  menu.Layout
	failure menu.ActionFailure
}

func newApp(cfg Config) *app {
	return &app{
		cfg:     cfg,
		refresh: make(chan struct{}, 1),
		actions: make(chan action, actionQueueSize),
		now:     time.Now,
	}
}

// watch runs onClick for every value on clicks until ctx ends.
func (a *app) watch(ctx context.Context, clicks <-chan struct{}, onClick func()) {
	for {
		select {
		case <-clicks:
			onClick()
		case <-ctx.Done():
			return
		}
	}
}

func (a *app) requestRefresh() {
	select {
	case a.refresh <- struct{}{}:
	default: // a refresh is already queued
	}
}

// poll runs one poll at a time on a single goroutine. A time.Ticker drops
// ticks while the receiver is busy, so a slow server cannot pile up requests.
func (a *app) poll(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.Interval)
	defer ticker.Stop()

	for {
		a.pollOnce(ctx)
		select {
		case <-ticker.C:
		case <-a.refresh:
		case <-ctx.Done():
			return
		}
	}
}

func (a *app) pollOnce(ctx context.Context) {
	models, err := a.cfg.Client.LoadedModels(ctx)

	now := a.now()
	layout := menu.LayoutFor(models, now)
	a.mu.Lock()
	a.models = models
	a.layout = layout
	failure := a.failure
	a.mu.Unlock()

	a.apply(applied{
		status:  status.From(models, err),
		tooltip: menu.Tooltip(a.cfg.Host, models, err, failure, now),
		layout:  layout,
	})
}

// apply sends only what differs from the last poll to the ui. Every ui call
// is a D-Bus signal, and an idle tray would otherwise send dozens per poll.
func (a *app) apply(next applied) {
	prev := a.last
	first := !a.hasLast
	a.last, a.hasLast = next, true

	if first || prev.status != next.status {
		a.ui.SetIcon(next.status)
	}
	if first || prev.tooltip != next.tooltip {
		a.ui.SetTooltip(next.tooltip)
	}
	for i, it := range next.layout.Items {
		prevItem := prev.layout.Items[i]
		if first || prevItem.Visible != it.Visible || prevItem.Label != it.Label {
			if it.Visible {
				a.ui.ShowModel(i, it.Label)
			} else {
				a.ui.HideModel(i)
			}
		}
		showKeep, prevShowKeep := it.Visible && it.KeepLoadedVisible, prevItem.Visible && prevItem.KeepLoadedVisible
		if first || showKeep != prevShowKeep || (showKeep && prevItem.KeepLabel != it.KeepLabel) {
			if showKeep {
				a.ui.ShowKeepLoaded(i, it.KeepLabel)
			} else {
				a.ui.HideKeepLoaded(i)
			}
		}
	}
	if first || prev.layout.KeepLoadedVisible != next.layout.KeepLoadedVisible {
		a.ui.SetKeepLoadedVisible(next.layout.KeepLoadedVisible)
	}
	if first || prev.layout.StopAllVisible != next.layout.StopAllVisible {
		a.ui.SetStopAllVisible(next.layout.StopAllVisible)
	}
}

// stopModel queues the unload of the model that item index showed at the
// last poll. It returns at once so the click handler keeps reading clicks.
func (a *app) stopModel(index int) {
	// The layout, not the live model list, names the model so the click
	// acts on what the item's label showed at the last poll.
	a.mu.Lock()
	name := a.layout.Items[index].Model
	a.mu.Unlock()
	if name == "" {
		return
	}
	a.enqueue(action{kind: actionStop, names: []string{name}})
}

// keepModel queues keeping the model that item index showed at the last poll
// loaded. A model that is already kept offers no such action, so a click on a
// stale item is ignored.
func (a *app) keepModel(index int) {
	a.mu.Lock()
	item := a.layout.Items[index]
	a.mu.Unlock()
	if item.Model == "" || !item.KeepLoadedVisible {
		return
	}
	a.enqueue(action{kind: actionKeepLoaded, names: []string{item.Model}})
}

// stopEveryModel queues the unload of every model from the last poll.
func (a *app) stopEveryModel() {
	a.mu.Lock()
	names := make([]string, 0, len(a.models))
	for _, m := range a.models {
		names = append(names, m.Name)
	}
	a.mu.Unlock()
	a.enqueue(action{kind: actionStop, names: names})
}

// enqueue never blocks the click handler. The queue holds more requests
// than a person can click while one runs, so a full queue is an edge case.
func (a *app) enqueue(act action) {
	select {
	case a.actions <- act:
	default:
		log.Printf("menu request dropped: %d requests already queued", actionQueueSize)
	}
}

// runActions runs queued requests one after another until ctx ends. A
// request waits here, not in the click handler, so no click is dropped
// while a slow server answers an earlier one.
func (a *app) runActions(ctx context.Context) {
	for {
		select {
		case act := <-a.actions:
			switch act.kind {
			case actionStop:
				a.unload(ctx, act.names...)
			case actionKeepLoaded:
				a.keepLoaded(ctx, act.names...)
			}
		case <-ctx.Done():
			return
		}
	}
}

// unload unloads names concurrently, so "Stop all" takes about one unload
// timeout instead of one per model.
func (a *app) unload(ctx context.Context, names ...string) {
	a.perform(ctx, "stop", a.cfg.Client.Unload, names)
}

func (a *app) keepLoaded(ctx context.Context, names ...string) {
	a.perform(ctx, "keep loaded", a.cfg.Client.KeepLoaded, names)
}

// perform calls call for each name concurrently, shows any failure in the
// tooltip under verb, and asks for a poll so the menu shows the result.
func (a *app) perform(ctx context.Context, verb string, call func(context.Context, string) error, names []string) {
	// A new attempt replaces the last failure, even before it finishes.
	a.mu.Lock()
	a.failure = menu.ActionFailure{}
	a.mu.Unlock()

	// Each goroutine writes only its own element, so no lock is needed.
	errs := make([]error, len(names))
	var group errgroup.Group
	group.SetLimit(maxConcurrentUnloads)
	for i, name := range names {
		group.Go(func() error {
			if err := call(ctx, name); err != nil {
				errs[i] = fmt.Errorf("%s: %w", name, err)
			}
			return nil
		})
	}
	_ = group.Wait() // every goroutine returns nil; failures are in errs

	err := errors.Join(errs...)
	if err != nil {
		log.Printf("%s failed: %v", verb, err)
	}

	a.mu.Lock()
	if err != nil {
		a.failure = menu.ActionFailure{Action: verb, Err: err, At: a.now()}
	}
	a.mu.Unlock()
	a.requestRefresh()
}
