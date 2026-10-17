package main

import (
	"bytes"
	"errors"
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
)

func TestParseConfig(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")

	tests := []struct {
		name         string
		args         []string
		wantHost     string
		wantInterval time.Duration
	}{
		{"defaults", nil, "http://127.0.0.1:11434", 5 * time.Second},
		{"flags", []string{"--host", "box:9", "--interval", "30s"}, "http://box:9", 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseConfig(tt.args, &bytes.Buffer{})
			require.NoError(t, err)
			assert.Equal(t, tt.wantHost, cfg.host)
			assert.Equal(t, tt.wantInterval, cfg.interval)
		})
	}
}

func TestParseConfigErrors(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "")

	tests := []struct {
		name    string
		args    []string
		wantErr error
	}{
		{"invalid port", []string{"--host", "box:notaport"}, ollama.ErrInvalidPort},
		{"zero interval", []string{"--interval", "0s"}, nil},
		{"unknown flag", []string{"--nope"}, errReported},
		{"help", []string{"-h"}, flag.ErrHelp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, &bytes.Buffer{})
			require.Error(t, err)
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr), "got %v", err)
			}
		})
	}
}
