// Package menu decides what the tray menu and tooltip show. It has no GUI
// imports, so it tests without a display.
package menu

import (
	"fmt"
	"strings"

	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
)

// PoolSize is how many "Stop <model>" items the tray creates up front.
// fyne.io/systray cannot remove items, so the tray builds a fixed pool and
// hides the unused ones. Models past the pool are still reachable with "Stop all".
const PoolSize = 8

// Item is one slot in the model pool.
type Item struct {
	Visible bool
	Label   string
	// Model is the name to unload when the item is clicked; empty for a hidden slot.
	Model string
}

// Layout is the visible state of the model part of the menu.
type Layout struct {
	Items          [PoolSize]Item
	StopAllVisible bool
}

// LayoutFor returns which pool items and "Stop all" show for the loaded models.
func LayoutFor(models []ollama.LoadedModel) Layout {
	var l Layout
	for i := range min(len(models), PoolSize) {
		l.Items[i] = Item{Visible: true, Label: "Stop " + models[i].Name, Model: models[i].Name}
	}
	l.StopAllVisible = len(models) > 1
	return l
}

// Tooltip describes the server address, each loaded model with its VRAM in
// GB, and the last failed stop, if any.
func Tooltip(host string, models []ollama.LoadedModel, pollErr, stopErr error) string {
	var b strings.Builder
	b.WriteString(host)
	switch {
	case pollErr != nil:
		b.WriteString("\nunreachable")
	case len(models) == 0:
		b.WriteString("\nno model loaded")
	default:
		for _, m := range models {
			fmt.Fprintf(&b, "\n%s (%.1f GB VRAM)", m.Name, float64(m.SizeVRAM)/1e9)
		}
	}
	if stopErr != nil {
		fmt.Fprintf(&b, "\nstop failed: %v", stopErr)
	}
	return b.String()
}
