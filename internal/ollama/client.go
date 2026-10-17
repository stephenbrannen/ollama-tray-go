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

const (
	// maxErrorBody caps how much of an error response is quoted in the error text.
	maxErrorBody = 512
	// maxResponseBody caps a success response. The host is user-configured, so
	// a wrong or hostile server must not make the tray buffer unbounded data.
	maxResponseBody = 1 << 20
	// maxDrainBody caps how much of an error response is read and discarded.
	// The transport reuses a connection only after the body is fully read.
	maxDrainBody = 64 << 10

	// statusTimeout bounds a status call, which runs on every poll.
	statusTimeout = 2 * time.Second
	// unloadTimeout is longer because the server may write a large model's
	// state out of memory before it answers.
	unloadTimeout = 10 * time.Second
)

// LoadedModel is a model currently held in memory by the server.
type LoadedModel struct {
	Name     string
	SizeVRAM int64 // bytes
	// ExpiresAt is when the server unloads the model on its own. The zero
	// value means the server sent no time, or one the client cannot read.
	ExpiresAt time.Time
}

// Client talks to one Ollama server.
type Client struct {
	baseURL       string
	http          *http.Client
	statusTimeout time.Duration
	unloadTimeout time.Duration
}

// NewHTTPClient returns the http.Client to pass to NewClient. It does not
// follow redirects: the host is user-configured, so a redirect could send the
// request to another host, or from https to http. A redirect response is
// returned as is and counts as ErrUnavailable. It sets no overall timeout
// because Client applies one per call.
func NewHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// NewClient returns a Client for baseURL (see ResolveHost).
func NewClient(baseURL string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:       strings.TrimRight(baseURL, "/"),
		http:          httpClient,
		statusTimeout: statusTimeout,
		unloadTimeout: unloadTimeout,
	}
}

type psResponse struct {
	Models []struct {
		Name     string `json:"name"`
		SizeVRAM int64  `json:"size_vram"`
		// ExpiresAt stays raw so that a format the server changes leaves the
		// time unknown instead of failing the whole poll.
		ExpiresAt json.RawMessage `json:"expires_at"`
	} `json:"models"`
}

// LoadedModels lists the models currently loaded (GET /api/ps).
func (c *Client) LoadedModels(ctx context.Context) ([]LoadedModel, error) {
	ctx, cancel := context.WithTimeout(ctx, c.statusTimeout)
	defer cancel()
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
		models = append(models, LoadedModel{Name: m.Name, SizeVRAM: m.SizeVRAM, ExpiresAt: parseExpiry(m.ExpiresAt)})
	}
	return models, nil
}

// parseExpiry reads an RFC 3339 time from a JSON string. Anything else gives
// the zero time, which means unknown.
func parseExpiry(raw json.RawMessage) time.Time {
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

type keepAliveRequest struct {
	Model string `json:"model"`
	// KeepAlive is how long the server keeps the model loaded after this
	// request. Zero tells it to unload now; it is the call `ollama stop` makes.
	// -1 tells it to keep the model until someone stops it.
	KeepAlive int `json:"keep_alive"`
}

// Unload evicts a model from memory (POST /api/generate with keep_alive 0).
func (c *Client) Unload(ctx context.Context, name string) error {
	return c.setKeepAlive(ctx, name, 0)
}

// KeepLoaded tells the server to hold a model in memory until it is stopped
// (POST /api/generate with keep_alive -1). It changes this model only, not
// the server's default keep-alive. A model that has already unloaded is
// loaded again first, because the server loads a model to answer the call.
func (c *Client) KeepLoaded(ctx context.Context, name string) error {
	return c.setKeepAlive(ctx, name, -1)
}

func (c *Client) setKeepAlive(ctx context.Context, name string, keepAlive int) error {
	payload, err := json.Marshal(keepAliveRequest{Model: name, KeepAlive: keepAlive})
	if err != nil {
		return fmt.Errorf("encode keep-alive request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.unloadTimeout)
	defer cancel()
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
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrainBody))
		return nil, fmt.Errorf("%w: %s %s: status %d: %s",
			ErrUnavailable, method, path, resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s %s: %w", ErrUnavailable, method, path, err)
	}
	return body, nil
}
