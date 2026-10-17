package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	expires := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		body string
		want []LoadedModel
	}{
		{name: "none", body: `{"models":[]}`, want: []LoadedModel{}},
		{
			name: "one",
			body: `{"models":[{"name":"llama3:8b","size_vram":5000,"expires_at":"2026-10-16T12:00:00Z"}]}`,
			want: []LoadedModel{{Name: "llama3:8b", SizeVRAM: 5000, ExpiresAt: expires}},
		},
		{
			name: "two",
			body: `{"models":[` +
				`{"name":"llama3:8b","size_vram":5000,"expires_at":"2026-10-16T12:00:00Z"},` +
				`{"name":"qwen:4b","size_vram":0,"expires_at":"2026-10-16T12:00:00Z"}]}`,
			want: []LoadedModel{
				{Name: "llama3:8b", SizeVRAM: 5000, ExpiresAt: expires},
				{Name: "qwen:4b", SizeVRAM: 0, ExpiresAt: expires},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			models, err := tt.client(t).LoadedModels(context.Background())

			require.ErrorIs(t, err, ErrUnavailable)
			assert.Nil(t, models)
		})
	}
}

func TestLoadedModelsHonorsContext(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("request should not be sent with a canceled context")
	}, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.LoadedModels(ctx)

	require.ErrorIs(t, err, ErrUnavailable)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestUnloadSendsKeepAliveZero(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = io.WriteString(w, `{"done":true}`)
	}, time.Second)

	err := client.Unload(context.Background(), "llama3:8b")

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/api/generate", gotPath)
	assert.JSONEq(t, `{"model":"llama3:8b","keep_alive":0}`, gotBody)
}

func TestUnloadUnavailable(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
	}, time.Second)

	err := client.Unload(context.Background(), "missing")

	require.ErrorIs(t, err, ErrUnavailable)
	assert.Contains(t, err.Error(), "model not found")
}
