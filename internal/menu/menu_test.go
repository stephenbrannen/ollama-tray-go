package menu_test

import (
	"errors"
	"fmt"
	"testing"

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
		{"more than the pool", models(many...), func() []string {
			out := make([]string, menu.PoolSize)
			for i := range out {
				out[i] = "Stop " + many[i]
			}
			return out
		}(), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			l := menu.LayoutFor(tt.models)
			assert.Equal(t, tt.wantVisible, visible(l))
			assert.Equal(t, tt.wantStopAll, l.StopAllVisible)
		})
	}
}

func TestLayoutForShrinkingHidesTrailingItems(t *testing.T) {
	t.Parallel()

	three := menu.LayoutFor(models("a", "b", "c"))
	one := menu.LayoutFor(models("a"))

	assert.Len(t, visible(three), 3)
	assert.Equal(t, []string{"Stop a"}, visible(one))
	assert.False(t, one.Items[1].Visible)
	assert.Empty(t, one.Items[1].Model, "a hidden slot must not keep a stale model name")
	assert.False(t, one.StopAllVisible)
}

func TestTooltip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		models  []ollama.LoadedModel
		pollErr error
		stopErr error
		want    string
	}{
		{
			"loaded", models("llama3", "qwen"), nil, nil,
			"http://h:11434\nllama3 (4.5 GB VRAM)\nqwen (4.5 GB VRAM)",
		},
		{"idle", nil, nil, nil, "http://h:11434\nno model loaded"},
		{"unreachable", nil, ollama.ErrUnavailable, nil, "http://h:11434\nunreachable"},
		{
			"stop failed", models("a"), nil, errors.New("boom"),
			"http://h:11434\na (4.5 GB VRAM)\nstop failed: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, menu.Tooltip("http://h:11434", tt.models, tt.pollErr, tt.stopErr))
		})
	}
}
