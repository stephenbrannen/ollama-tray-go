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

// PoolSize is how many model submenus the tray creates up front.
// fyne.io/systray cannot remove items, so the tray builds a fixed pool and
// hides the unused ones. Models past the pool are still reachable with "Stop all".
const PoolSize = 8

// Labels of the two actions in each model submenu.
const (
	StopLabel       = "Stop"
	KeepLoadedLabel = "Keep loaded"
)

const (
	// ActionFailureTTL is how long a failed action stays in the tooltip.
	ActionFailureTTL = 60 * time.Second

	// keptLoadedAfter separates a model kept forever from one with a long
	// keep-alive. For keep-alive -1 the server reports a time about 290 years
	// away (the year 2319 when checked), not a special value.
	keptLoadedAfter = 100 * 365 * 24 * time.Hour

	maxNameRunes  = 64
	maxErrorRunes = 200
)

// ActionFailure is the last failed menu action and when it happened. Action
// is the verb the tooltip shows, such as "stop". The zero value means no failure.
type ActionFailure struct {
	Action string
	Err    error
	At     time.Time
}

// Item is one slot in the model pool.
type Item struct {
	Visible bool
	// Label is the submenu title, the model name.
	Label string
	// Model is the name the submenu actions apply to; empty for a hidden slot.
	Model string
	// KeepLoadedVisible is false once the server already keeps the model forever.
	KeepLoadedVisible bool
}

// Layout is the visible state of the model part of the menu.
type Layout struct {
	Items          [PoolSize]Item
	StopAllVisible bool
}

// LayoutFor returns which pool items and "Stop all" show for the loaded models.
func LayoutFor(models []ollama.LoadedModel, now time.Time) Layout {
	var l Layout
	for i := range min(len(models), PoolSize) {
		l.Items[i] = Item{
			Visible:           true,
			Label:             clean(models[i].Name, maxNameRunes),
			Model:             models[i].Name,
			KeepLoadedVisible: !keptLoaded(models[i].ExpiresAt, now),
		}
	}
	l.StopAllVisible = len(models) > 1
	return l
}

// Tooltip describes the server address, each loaded model with its VRAM in
// GB and the time until it unloads, and the last failed action while it is less than ActionFailureTTL old.
func Tooltip(host string, models []ollama.LoadedModel, pollErr error, failure ActionFailure, now time.Time) string {
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
	if failure.Err != nil && now.Sub(failure.At) < ActionFailureTTL {
		fmt.Fprintf(&b, "\n%s failed: %s", failure.Action, clean(failure.Err.Error(), maxErrorRunes))
	}
	return b.String()
}

// keptLoaded reports whether the server keeps a model until it is stopped.
func keptLoaded(expiresAt, now time.Time) bool {
	return !expiresAt.IsZero() && expiresAt.Sub(now) > keptLoadedAfter
}

// unloadText says when the server unloads a model on its own. It is empty
// when the time is unknown, so the tooltip shows nothing it cannot back up.
func unloadText(expiresAt, now time.Time) string {
	if expiresAt.IsZero() {
		return ""
	}
	left := expiresAt.Sub(now)
	switch {
	case keptLoaded(expiresAt, now):
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
