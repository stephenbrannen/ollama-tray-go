// Package menu decides what the tray menu and tooltip show. It has no GUI
// imports, so it tests without a display.
package menu

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
)

// PoolSize is how many "Stop <model>" items the tray creates up front.
// fyne.io/systray cannot remove items, so the tray builds a fixed pool and
// hides the unused ones. Models past the pool are still reachable with "Stop all".
const PoolSize = 8

const (
	// StopFailureTTL is how long a failed stop stays in the tooltip.
	StopFailureTTL = 60 * time.Second

	// keptLoadedAfter separates a model kept forever from one with a long
	// keep-alive. For keep-alive -1 the server reports a time about 290 years
	// away (the year 2319 when checked), not a special value.
	keptLoadedAfter = 100 * 365 * 24 * time.Hour

	maxNameRunes  = 64
	maxErrorRunes = 200
)

// StopFailure is the last failed stop and when it happened. The zero value
// means no failure.
type StopFailure struct {
	Err error
	At  time.Time
}

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
		l.Items[i] = Item{Visible: true, Label: "Stop " + clean(models[i].Name, maxNameRunes), Model: models[i].Name}
	}
	l.StopAllVisible = len(models) > 1
	return l
}

// Tooltip describes the server address, each loaded model with its VRAM in
// GB and the time until it unloads, and the last failed stop while it is less than StopFailureTTL old.
func Tooltip(host string, models []ollama.LoadedModel, pollErr error, stop StopFailure, now time.Time) string {
	var b strings.Builder
	b.WriteString(host)
	switch {
	case pollErr != nil:
		b.WriteString("\nunreachable")
	case len(models) == 0:
		b.WriteString("\nno model loaded")
	default:
		for _, m := range models {
			fmt.Fprintf(&b, "\n%s (%.1f GB VRAM", clean(m.Name, maxNameRunes), float64(m.SizeVRAM)/1e9)
			if text := unloadText(m.ExpiresAt, now); text != "" {
				b.WriteString(", " + text)
			}
			b.WriteString(")")
		}
	}
	if stop.Err != nil && now.Sub(stop.At) < StopFailureTTL {
		fmt.Fprintf(&b, "\nstop failed: %s", clean(stop.Err.Error(), maxErrorRunes))
	}
	return b.String()
}

// unloadText says when the server unloads a model on its own. It is empty
// when the time is unknown, so the tooltip shows nothing it cannot back up.
func unloadText(expiresAt, now time.Time) string {
	if expiresAt.IsZero() {
		return ""
	}
	left := expiresAt.Sub(now)
	switch {
	case left > keptLoadedAfter:
		return "kept loaded"
	case left < time.Minute:
		return "unloads in <1m"
	case left < time.Hour:
		return fmt.Sprintf("unloads in %dm", int(left.Minutes()))
	}
	hours, minutes := int(left.Hours()), int(left.Minutes())%60
	if minutes == 0 {
		return fmt.Sprintf("unloads in %dh", hours)
	}
	return fmt.Sprintf("unloads in %dh %dm", hours, minutes)
}

// CheckingTooltip is the tooltip before the first poll returns.
func CheckingTooltip(host string) string {
	return host + "\nchecking"
}

// clean makes text from the server safe for a tooltip or menu label. A model
// name or error message can hold newlines or terminal escape sequences, and
// the tray shows them as given. Control and invisible format characters
// become spaces, and text past limit characters is cut.
func clean(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if runes := []rune(s); len(runes) > limit {
		return string(runes[:limit-1]) + "…"
	}
	return s
}
