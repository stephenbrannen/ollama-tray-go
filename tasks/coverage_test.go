//go:build task

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPercent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile string
		want    float64
		wantErr bool
	}{
		{
			name:    "counts statements in hit blocks",
			profile: "mode: atomic\na.go:1.1,2.2 3 1\na.go:3.1,4.2 1 0\n",
			want:    75,
		},
		{
			name:    "a block counts as covered when any test binary ran it",
			profile: "mode: atomic\na.go:1.1,2.2 2 5\na.go:1.1,2.2 2 0\na.go:3.1,4.2 2 0\n",
			want:    50,
		},
		{
			name:    "empty profile",
			profile: "mode: atomic\n",
			wantErr: true,
		},
		{
			name:    "malformed line",
			profile: "mode: atomic\na.go:1.1,2.2 x 1\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "cover.out")
			require.NoError(t, os.WriteFile(path, []byte(tt.profile), 0o600))

			got, err := percent(path)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestIsWiring(t *testing.T) {
	t.Parallel()

	assert.True(t, isWiring("github.com/stephenbrannen/ollama-tray-go/cmd/ollama-tray"))
	assert.True(t, isWiring("github.com/stephenbrannen/ollama-tray-go/internal/tray"))
	assert.False(t, isWiring("github.com/stephenbrannen/ollama-tray-go/internal/ollama"))
}
