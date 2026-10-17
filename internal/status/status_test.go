package status_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
	"github.com/stephenbrannen/ollama-tray-go/internal/status"
)

func TestFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		models []ollama.LoadedModel
		err    error
		want   status.Status
	}{
		{"one model", []ollama.LoadedModel{{Name: "llama3"}}, nil, status.Loaded},
		{"two models", []ollama.LoadedModel{{Name: "a"}, {Name: "b"}}, nil, status.Loaded},
		{"nil list", nil, nil, status.Idle},
		{"empty list", []ollama.LoadedModel{}, nil, status.Idle},
		{"unavailable", nil, ollama.ErrUnavailable, status.Unreachable},
		{"other error", nil, errors.New("boom"), status.Unreachable},
		{"error wins over models", []ollama.LoadedModel{{Name: "a"}}, ollama.ErrUnavailable, status.Unreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, status.From(tt.models, tt.err))
		})
	}
}
