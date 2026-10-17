package main

import (
	"bytes"
	"strings"
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
		{"minimum interval", []string{"--interval", "1s"}, "http://127.0.0.1:11434", time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseConfig(tt.args, &bytes.Buffer{})
			require.NoError(t, err)
			assert.Equal(t, config{host: tt.wantHost, interval: tt.wantInterval}, res.cfg)
			assert.False(t, res.help)
			assert.False(t, res.version)
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
		{"invalid scheme", []string{"--host", "ftp://box"}, ollama.ErrInvalidHost},
		{"zero interval", []string{"--interval", "0s"}, errInvalidInterval},
		{"negative interval", []string{"--interval", "-5s"}, errInvalidInterval},
		{"interval below minimum", []string{"--interval", "999ms"}, errInvalidInterval},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseConfig(tt.args, &bytes.Buffer{})
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestParseConfigUnknownFlag(t *testing.T) {
	var stderr bytes.Buffer

	_, err := parseConfig([]string{"--nope"}, &stderr)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "-nope")
	assert.Contains(t, stderr.String(), "Usage of ollama-tray:")
}

func TestParseConfigHelpAndVersion(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		want      parseResult
		wantUsage bool
	}{
		{"help", []string{"-h"}, parseResult{help: true}, true},
		{"version", []string{"--version"}, parseResult{version: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer

			res, err := parseConfig(tt.args, &stderr)

			require.NoError(t, err)
			assert.Equal(t, tt.want, res)
			assert.Equal(t, tt.wantUsage, strings.Contains(stderr.String(), "Usage of ollama-tray:"))
		})
	}
}
