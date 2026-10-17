// Package ollama is a small client for the two Ollama endpoints the tray uses.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnavailable means the server cannot be reached or answered with an error.
// It wraps connection errors, timeouts and non-2xx responses.
var ErrUnavailable = errors.New("ollama server unavailable")

// maxErrorBody caps how much of an error response is quoted in the error text.
const maxErrorBody = 512

// LoadedModel is a model currently held in memory by the server.
type LoadedModel struct {
	Name      string
	SizeVRAM  int64 // bytes
	ExpiresAt time.Time
}

// Client talks to one Ollama server.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient returns a Client for baseURL (see ResolveHost). Per-call
// timeouts belong on httpClient or on the context.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type psResponse struct {
	Models []struct {
		Name      string    `json:"name"`
		SizeVRAM  int64     `json:"size_vram"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"models"`
}

// LoadedModels lists the models currently loaded (GET /api/ps).
func (c *Client) LoadedModels(ctx context.Context) ([]LoadedModel, error) {
	body, err := c.do(ctx, http.MethodGet, "/api/ps", nil)
	if err != nil {
		return nil, err
	}

	var parsed psResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode /api/ps: %w", ErrUnavailable, err)
	}

	models := make([]LoadedModel, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		models = append(models, LoadedModel{Name: m.Name, SizeVRAM: m.SizeVRAM, ExpiresAt: m.ExpiresAt})
	}
	return models, nil
}

type unloadRequest struct {
	Model string `json:"model"`
	// KeepAlive is how long the server keeps the model loaded after this
	// request. Zero tells it to unload now; it is the call `ollama stop` makes.
	KeepAlive int `json:"keep_alive"`
}

// Unload evicts a model from memory (POST /api/generate with keep_alive 0).
func (c *Client) Unload(ctx context.Context, name string) error {
	payload, err := json.Marshal(unloadRequest{Model: name, KeepAlive: 0})
	if err != nil {
		return fmt.Errorf("encode unload request: %w", err)
	}
	_, err = c.do(ctx, http.MethodPost, "/api/generate", payload)
	return err
}

func (c *Client) do(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("build %s %s: %w", method, path, err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s: %w", ErrUnavailable, method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("%w: %s %s: status %d: %s",
			ErrUnavailable, method, path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s %s: %w", ErrUnavailable, method, path, err)
	}
	return body, nil
}
