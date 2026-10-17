package menu_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stephenbrannen/ollama-tray-go/internal/menu"
	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
)

func models(names ...string) []ollama.LoadedModel {
	out := make([]ollama.LoadedModel, 0, len(names))
	for _, n := range names {
		out = append(out, ollama.LoadedModel{Name: n, SizeVRAM: 4_500_000_000})
	}
	return out
}

func visible(l menu.Layout) []string {
	var labels []string
	for _, it := range l.Items {
		if it.Visible {
			labels = append(labels, it.Label)
		}
	}
	return labels
}

func stops(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "Stop " + n
	}
	return out
}

func TestLayoutFor(t *testing.T) {
	t.Parallel()

	many := make([]string, menu.PoolSize+2)
	for i := range many {
		many[i] = fmt.Sprintf("m%d", i)
	}

	tests := []struct {
		name        string
		models      []ollama.LoadedModel
		wantVisible []string
		wantStopAll bool
	}{
		{"none", nil, nil, false},
		{"one", models("a"), []string{"Stop a"}, false},
		{"three", models("a", "b", "c"), []string{"Stop a", "Stop b", "Stop c"}, true},
		{"exactly the pool", models(many[:menu.PoolSize]...), stops(many[:menu.PoolSize]), true},
		{"one past the pool", models(many[:menu.PoolSize+1]...), stops(many[:menu.PoolSize]), true},
		{"more than the pool", models(many...), stops(many[:menu.PoolSize]), true},
		{"unsafe name", models("a\nb\x1b[31m"), []string{"Stop a b [31m"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := menu.LayoutFor(tt.models, time.Time{})
			assert.Equal(t, tt.wantVisible, visible(l))
			assert.Equal(t, tt.wantStopAll, l.StopAllVisible)
		})
	}
}

func TestLayoutForOffersKeepLoadedUnlessAlreadyKept(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"unknown expiry", time.Time{}, true},
		{"unloads soon", now.Add(5 * time.Minute), true},
		{"long keep-alive", now.Add(30 * 24 * time.Hour), true},
		{"kept forever", time.Date(2319, 1, 27, 2, 29, 9, 0, time.UTC), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loaded := []ollama.LoadedModel{{Name: "a", ExpiresAt: tt.expiresAt}}
			layout := menu.LayoutFor(loaded, now)
			assert.Equal(t, tt.want, layout.Items[0].KeepLoadedVisible)
			assert.Equal(t, tt.want, layout.KeepLoadedVisible)
			assert.Equal(t, "a", layout.Items[0].KeepLabel)
		})
	}
}

func TestLayoutForKeepLoadedSubmenuShowsWhileAnyModelCanBeKept(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	forever := time.Date(2319, 1, 27, 2, 29, 9, 0, time.UTC)
	tests := []struct {
		name   string
		models []ollama.LoadedModel
		want   bool
	}{
		{"no model", nil, false},
		{"one kept, one not", []ollama.LoadedModel{{Name: "a", ExpiresAt: forever}, {Name: "b"}}, true},
		{"all kept", []ollama.LoadedModel{{Name: "a", ExpiresAt: forever}, {Name: "b", ExpiresAt: forever}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, menu.LayoutFor(tt.models, now).KeepLoadedVisible)
		})
	}
}

func TestLayoutForShrinkingHidesTrailingItems(t *testing.T) {
	t.Parallel()

	three := menu.LayoutFor(models("a", "b", "c"), time.Time{})
	one := menu.LayoutFor(models("a"), time.Time{})

	assert.Len(t, visible(three), 3)
	assert.Equal(t, []string{"Stop a"}, visible(one))
	assert.False(t, one.Items[1].Visible)
	assert.Empty(t, one.Items[1].Model, "a hidden slot must not keep a stale model name")
	assert.False(t, one.StopAllVisible)
}

func TestTooltip(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	failedAt := func(ago time.Duration) menu.ActionFailure {
		return menu.ActionFailure{Action: "stop", Err: errors.New("boom"), At: now.Add(-ago)}
	}
	tests := []struct {
		name    string
		models  []ollama.LoadedModel
		pollErr error
		stop    menu.ActionFailure
		want    string
	}{
		{
			"loaded", models("llama3", "qwen"), nil,
			menu.ActionFailure{},
			"http://h:11434\nllama3 (4.5 GB VRAM)\nqwen (4.5 GB VRAM)",
		},
		{"idle", nil, nil, menu.ActionFailure{}, "http://h:11434\nno model loaded"},
		{"unreachable", nil, ollama.ErrUnavailable, menu.ActionFailure{}, "http://h:11434\nunreachable"},
		{
			"fresh stop failure", models("a"), nil, failedAt(time.Second),
			"http://h:11434\na (4.5 GB VRAM)\nstop failed: boom",
		},
		{
			"fresh keep loaded failure", models("a"), nil,
			menu.ActionFailure{Action: "keep loaded", Err: errors.New("boom"), At: now.Add(-time.Second)},
			"http://h:11434\na (4.5 GB VRAM)\nkeep loaded failed: boom",
		},
		{
			"stop failure just inside the limit", nil, nil, failedAt(menu.ActionFailureTTL - time.Nanosecond),
			"http://h:11434\nno model loaded\nstop failed: boom",
		},
		{
			"stop failure at the limit", models("a"), nil, failedAt(menu.ActionFailureTTL),
			"http://h:11434\na (4.5 GB VRAM)",
		},
		{
			"expired stop failure", models("a"), nil, failedAt(time.Hour),
			"http://h:11434\na (4.5 GB VRAM)",
		},
		{
			"joined stop errors on one line", nil, nil,
			menu.ActionFailure{Action: "stop", Err: errors.Join(errors.New("a: boom"), errors.New("c: boom")), At: now},
			"http://h:11434\nno model loaded\nstop failed: a: boom c: boom",
		},
		{
			"unsafe model name", models("evil\nstop failed: x\x07"), nil,
			menu.ActionFailure{},
			"http://h:11434\nevil stop failed: x (4.5 GB VRAM)",
		},
		{
			"long model name is cut", models(strings.Repeat("m", 100)), nil,
			menu.ActionFailure{},
			"http://h:11434\n" + strings.Repeat("m", 63) + "\u2026 (4.5 GB VRAM)",
		},
		{
			"long error is cut", nil, nil,
			menu.ActionFailure{Action: "stop", Err: errors.New(strings.Repeat("e", 300)), At: now},
			"http://h:11434\nno model loaded\nstop failed: " + strings.Repeat("e", 199) + "\u2026",
		},
		{
			"invalid utf-8 is dropped", models("a\xffb"), nil,
			menu.ActionFailure{},
			"http://h:11434\nab (4.5 GB VRAM)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, menu.Tooltip("http://h:11434", tt.models, tt.pollErr, tt.stop, now))
		})
	}
}

func TestTooltipUnloadTime(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	tests := []struct {
		name      string
		expiresAt time.Time
		want      string
	}{
		{"unknown", time.Time{}, "http://h:11434\nllama3 (4.5 GB VRAM)"},
		{"already past", now.Add(-time.Minute), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in <1m)"},
		{"seconds", now.Add(59 * time.Second), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in <1m)"},
		{"one minute", now.Add(time.Minute), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 1m)"},
		{"minutes round down", now.Add(9*time.Minute + 59*time.Second), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 9m)"},
		{"just under an hour", now.Add(time.Hour - time.Second), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 59m)"},
		{"whole hours", now.Add(2 * time.Hour), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 2h)"},
		{"hours and minutes", now.Add(3*time.Hour + 12*time.Minute + 30*time.Second), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 3h 12m)"},
		{"days stay in hours", now.Add(72 * time.Hour), "http://h:11434\nllama3 (4.5 GB VRAM, unloads in 72h)"},
		{"forever", time.Date(2319, 1, 27, 2, 29, 9, 0, time.UTC), "http://h:11434\nllama3 (4.5 GB VRAM, kept loaded)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loaded := []ollama.LoadedModel{{Name: "llama3", SizeVRAM: 4_500_000_000, ExpiresAt: tt.expiresAt}}
			assert.Equal(t, tt.want, menu.Tooltip("http://h:11434", loaded, nil, menu.ActionFailure{}, now))
		})
	}
}

func TestCheckingTooltip(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "http://h:11434\nchecking", menu.CheckingTooltip("http://h:11434"))
}
