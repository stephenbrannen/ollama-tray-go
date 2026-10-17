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

	"fyne.io/systray"

	"github.com/stephenbrannen/ollama-tray-go/internal/icon"
	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

// requestTimeout bounds each status and unload call.
const requestTimeout = 2 * time.Second

// Server is the part of the Ollama client the tray uses.
type Server interface {
	LoadedModels(ctx context.Context) ([]ollama.LoadedModel, error)
	Unload(ctx context.Context, name string) error
}

// Config is what Run needs from the command line.
type Config struct {
	Client   Server
	Host     string
	Interval time.Duration
}

type app struct {
	cfg     Config
	refresh chan struct{}
	items   [menu.PoolSize]*systray.MenuItem
	stopAll *systray.MenuItem

	mu      sync.Mutex
	models  []ollama.LoadedModel
	layout  menu.Layout
	stopErr error
}

// Run shows the tray icon and blocks until the user quits or ctx is cancelled.
func Run(ctx context.Context, cfg Config) {
	a := &app{cfg: cfg, refresh: make(chan struct{}, 1)}
	systray.Run(func() { a.onReady(ctx) }, func() {})
}

func (a *app) onReady(ctx context.Context) {
	systray.SetIcon(icon.PNG(status.Unreachable))
	systray.SetTitle("Ollama")
	systray.SetTooltip(a.cfg.Host)

	for i := range a.items {
		a.items[i] = systray.AddMenuItem("", "")
		a.items[i].Hide()
		go a.watch(ctx, a.items[i], func() { a.stopModel(ctx, i) })
	}
	a.stopAll = systray.AddMenuItem("Stop all", "Unload every loaded model")
	a.stopAll.Hide()
	go a.watch(ctx, a.stopAll, func() { a.stopEveryModel(ctx) })

	systray.AddSeparator()
	refresh := systray.AddMenuItem("Refresh now", "")
	go a.watch(ctx, refresh, a.requestRefresh)
	quit := systray.AddMenuItem("Quit", "")
	go func() {
		select {
		case <-quit.ClickedCh:
		case <-ctx.Done():
		}
		systray.Quit()
	}()

	go a.poll(ctx)
}

// watch runs onClick for every click on item until ctx ends.
func (a *app) watch(ctx context.Context, item *systray.MenuItem, onClick func()) {
	for {
		select {
		case <-item.ClickedCh:
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
	callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	models, err := a.cfg.Client.LoadedModels(callCtx)

	layout := menu.LayoutFor(models)
	a.mu.Lock()
	a.models = models
	a.layout = layout
	stopErr := a.stopErr
	a.mu.Unlock()

	systray.SetIcon(icon.PNG(status.From(models, err)))
	systray.SetTooltip(menu.Tooltip(a.cfg.Host, models, err, stopErr))

	for i, it := range layout.Items {
		if it.Visible {
			a.items[i].SetTitle(it.Label)
			a.items[i].Show()
		} else {
			a.items[i].Hide()
		}
	}
	if layout.StopAllVisible {
		a.stopAll.Show()
	} else {
		a.stopAll.Hide()
	}
}

func (a *app) stopModel(ctx context.Context, index int) {
	// The layout, not the live model list, names the model so the click
	// unloads what the item's label showed at the last poll.
	a.mu.Lock()
	name := a.layout.Items[index].Model
	a.mu.Unlock()
	if name == "" {
		return
	}
	a.unload(ctx, name)
}

func (a *app) stopEveryModel(ctx context.Context) {
	a.mu.Lock()
	names := make([]string, 0, len(a.models))
	for _, m := range a.models {
		names = append(names, m.Name)
	}
	a.mu.Unlock()
	a.unload(ctx, names...)
}

func (a *app) unload(ctx context.Context, names ...string) {
	var errs []error
	for _, name := range names {
		callCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		if err := a.cfg.Client.Unload(callCtx, name); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
		cancel()
	}
	err := errors.Join(errs...)
	if err != nil {
		log.Printf("stop failed: %v", err)
	}

	a.mu.Lock()
	a.stopErr = err
	a.mu.Unlock()
	a.requestRefresh()
}
