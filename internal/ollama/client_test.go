package ollama

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, timeout time.Duration) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, &http.Client{Timeout: timeout})
}

func TestLoadedModelsParsesResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []LoadedModel
	}{
		{name: "none", body: `{"models":[]}`, want: []LoadedModel{}},
		{
			name: "one",
			body: `{"models":[{"name":"llama3:8b","size_vram":5000,"expires_at":"2026-10-16T12:00:00Z"}]}`,
			want: []LoadedModel{{Name: "llama3:8b", SizeVRAM: 5000, ExpiresAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}},
		},
		{
			name: "fractional seconds and zone offset",
			body: `{"models":[{"name":"a","expires_at":"2026-10-16T08:00:00.123456789-04:00"}]}`,
			want: []LoadedModel{{Name: "a", ExpiresAt: time.Date(2026, 10, 16, 12, 0, 0, 123456789, time.UTC)}},
		},
		{
			name: "keep alive forever",
			body: `{"models":[{"name":"a","expires_at":"2319-01-27T02:29:09.005268606Z"}]}`,
			want: []LoadedModel{{Name: "a", ExpiresAt: time.Date(2319, 1, 27, 2, 29, 9, 5268606, time.UTC)}},
		},
		{
			name: "unreadable time is unknown",
			body: `{"models":[{"name":"a","expires_at":"next tuesday"}]}`,
			want: []LoadedModel{{Name: "a"}},
		},
		{
			name: "time of the wrong type is unknown",
			body: `{"models":[{"name":"a","expires_at":12345}]}`,
			want: []LoadedModel{{Name: "a"}},
		},
		{
			name: "missing time is unknown",
			body: `{"models":[{"name":"a"}]}`,
			want: []LoadedModel{{Name: "a"}},
		},
		{
			name: "two",
			body: `{"models":[` +
				`{"name":"llama3:8b","size_vram":5000,"expires_at":"2026-10-16T12:00:00Z"},` +
				`{"name":"qwen:4b","size_vram":0,"expires_at":"2026-10-16T12:00:00Z"}]}`,
			want: []LoadedModel{
				{Name: "llama3:8b", SizeVRAM: 5000, ExpiresAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)},
				{Name: "qwen:4b", SizeVRAM: 0, ExpiresAt: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/api/ps", r.URL.Path)
				_, _ = io.WriteString(w, tt.body)
			}, time.Second)

			got, err := client.LoadedModels(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoadedModelsUnavailable(t *testing.T) {
	t.Parallel()

	refused := httptest.NewServer(http.NotFoundHandler())
	refusedURL := refused.URL
	refused.Close()

	tests := []struct {
		name   string
		client func(t *testing.T) *Client
	}{
		{
			name: "connection refused",
			client: func(*testing.T) *Client {
				return NewClient(refusedURL, &http.Client{Timeout: time.Second})
			},
		},
		{
			name: "timeout",
			client: func(t *testing.T) *Client {
				return newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
					<-r.Context().Done()
				}, 50*time.Millisecond)
			},
		},
		{
			name: "http 500",
			client: func(t *testing.T) *Client {
				return newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "boom", http.StatusInternalServerError)
				}, time.Second)
			},
		},
		{
			name: "malformed json",
			client: func(t *testing.T) *Client {
				return newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, "not json")
				}, time.Second)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			models, err := tt.client(t).LoadedModels(context.Background())

			require.ErrorIs(t, err, ErrUnavailable)
			assert.Nil(t, models)
		})
	}
}

func TestLoadedModelsHonorsContext(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("request should not be sent with a canceled context")
	}, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.LoadedModels(ctx)

	require.ErrorIs(t, err, ErrUnavailable)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestKeepAliveCallsSendTheExactRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		call     func(*Client) error
		wantBody string
	}{
		{"unload sends keep_alive 0", func(c *Client) error { return c.Unload(context.Background(), "llama3:8b") }, `{"model":"llama3:8b","keep_alive":0}`},
		{"keep loaded sends keep_alive -1", func(c *Client) error { return c.KeepLoaded(context.Background(), "llama3:8b") }, `{"model":"llama3:8b","keep_alive":-1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var gotMethod, gotPath, gotBody string
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				_, _ = io.WriteString(w, `{"done":true}`)
			}, time.Second)

			err := tt.call(client)

			require.NoError(t, err)
			assert.Equal(t, http.MethodPost, gotMethod)
			assert.Equal(t, "/api/generate", gotPath)
			assert.JSONEq(t, tt.wantBody, gotBody)
		})
	}
}

func TestUnloadUnavailable(t *testing.T) {
	t.Parallel()

	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}, time.Second)

	err := client.Unload(context.Background(), "missing")

	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "model not found")
}

func TestNewHTTPClientDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	var targetHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetHits.Add(1)
		_, _ = io.WriteString(w, `{"models":[]}`)
	}))
	t.Cleanup(target.Close)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL+r.URL.Path)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(redirector.Close)
	client := NewClient(redirector.URL, NewHTTPClient())

	models, err := client.LoadedModels(context.Background())

	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "status 302")
	assert.Nil(t, models)
	assert.Zero(t, targetHits.Load())
}

func TestCallTimeouts(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 2*time.Second, NewClient("http://box", nil).statusTimeout)
	assert.Equal(t, 10*time.Second, NewClient("http://box", nil).unloadTimeout)

	short := 50 * time.Millisecond
	tests := []struct {
		name          string
		statusTimeout time.Duration
		unloadTimeout time.Duration
		call          func(c *Client) error
	}{
		{"status", short, time.Hour, func(c *Client) error { _, err := c.LoadedModels(context.Background()); return err }},
		{"unload", time.Hour, short, func(c *Client) error { return c.Unload(context.Background(), "llama3:8b") }},
		{"keep loaded", time.Hour, short, func(c *Client) error { return c.KeepLoaded(context.Background(), "llama3:8b") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t, func(_ http.ResponseWriter, r *http.Request) {
				// The server notices a closed client only after it reads the request body.
				_, _ = io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			}, 5*time.Second)
			client.statusTimeout = tt.statusTimeout
			client.unloadTimeout = tt.unloadTimeout

			err := tt.call(client)

			require.ErrorIs(t, err, ErrUnavailable)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
		})
	}
}

// This test is not parallel: httptest.Server.Close closes the idle
// connections of http.DefaultTransport, which NewHTTPClient uses, so a
// parallel test finishing would break the reuse being measured.
func TestErrorResponseBodyIsDrainedSoConnectionIsReused(t *testing.T) {
	var newConns atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, strings.Repeat("x", 8*maxErrorBody))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConns.Add(1)
		}
	}
	server.Start()
	t.Cleanup(server.Close)
	client := NewClient(server.URL, NewHTTPClient())

	for range 3 {
		_, err := client.LoadedModels(context.Background())
		require.ErrorIs(t, err, ErrUnavailable)
	}

	assert.Equal(t, int32(1), newConns.Load())
}
