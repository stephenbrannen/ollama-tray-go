package tray

import (
	"context"

	"fyne.io/systray"

	"github.com/stephenbrannen/ollama-tray-go/internal/icon"
	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

// systrayUI is the ui that draws on the real system tray.
type systrayUI struct {
	items   [menu.PoolSize]*systray.MenuItem
	stopAll *systray.MenuItem
}

func (u *systrayUI) SetIcon(s status.Status) { systray.SetIcon(icon.PNG(s)) }

func (u *systrayUI) SetTooltip(text string) { systray.SetTooltip(text) }

func (u *systrayUI) ShowModel(slot int, title string) {
	u.items[slot].SetTitle(title)
	u.items[slot].Show()
}

func (u *systrayUI) HideModel(slot int) { u.items[slot].Hide() }

func (u *systrayUI) SetStopAllVisible(visible bool) {
	if visible {
		u.stopAll.Show()
	} else {
		u.stopAll.Hide()
	}
}

// Run shows the tray icon and blocks until the user quits or ctx is cancelled.
func Run(ctx context.Context, cfg Config) {
	a := newApp(cfg)
	systray.Run(func() { a.onReady(ctx) }, func() {})
}

func (a *app) onReady(ctx context.Context) {
	u := &systrayUI{}
	u.SetIcon(status.Unreachable)
	systray.SetTitle("Ollama")
	u.SetTooltip(a.cfg.Host)

	for i := range u.items {
		u.items[i] = systray.AddMenuItem("", "")
		u.items[i].Hide()
		go a.watch(ctx, u.items[i].ClickedCh, func() { a.stopModel(i) })
	}
	u.stopAll = systray.AddMenuItem("Stop all", "Unload every loaded model")
	u.stopAll.Hide()
	go a.watch(ctx, u.stopAll.ClickedCh, a.stopEveryModel)

	systray.AddSeparator()
	refresh := systray.AddMenuItem("Refresh now", "")
	go a.watch(ctx, refresh.ClickedCh, a.requestRefresh)
	quit := systray.AddMenuItem("Quit", "")
	go func() {
		select {
		case <-quit.ClickedCh:
		case <-ctx.Done():
		}
		systray.Quit()
	}()

	a.ui = u
	go a.runStops(ctx)
	go a.poll(ctx)
}
