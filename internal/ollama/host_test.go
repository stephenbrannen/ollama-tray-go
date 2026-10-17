package ollama

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveHost(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
		err  error
	}{
		{name: "default", want: "http://127.0.0.1:11434"},
		{name: "env used when no flag", env: "10.0.0.5:9999", want: "http://10.0.0.5:9999"},
		{name: "flag beats env", flag: "flaghost:1", env: "envhost:2", want: "http://flaghost:1"},
		{name: "bare host gets scheme and port", flag: "gpu-box", want: "http://gpu-box:11434"},
		{name: "host and port gets scheme", flag: "gpu-box:8080", want: "http://gpu-box:8080"},
		{name: "http scheme without port", flag: "http://gpu-box", want: "http://gpu-box:11434"},
		{name: "https scheme without port", flag: "https://gpu-box", want: "https://gpu-box:11434"},
		{name: "https with port kept", flag: "https://gpu-box:8443", want: "https://gpu-box:8443"},
		{name: "trailing slash trimmed", flag: "http://gpu-box:8080/", want: "http://gpu-box:8080"},
		{name: "port only", flag: ":8080", want: "http://127.0.0.1:8080"},
		{name: "ipv6 without port", flag: "[::1]", want: "http://[::1]:11434"},
		{name: "ipv6 with port", flag: "[::1]:9000", want: "http://[::1]:9000"},
		{name: "path kept", flag: "http://box:8080/ollama/", want: "http://box:8080/ollama"},
		{name: "path kept without port", flag: "https://box/a/b", want: "https://box:11434/a/b"},
		{name: "non-numeric port", flag: "gpu-box:abc", err: ErrInvalidPort},
		{name: "port above range", flag: "gpu-box:70000", err: ErrInvalidPort},
		{name: "port zero", flag: "gpu-box:0", err: ErrInvalidPort},
		{name: "blank flag falls back to env", flag: "  ", env: "envhost", want: "http://envhost:11434"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OLLAMA_HOST", tt.env)

			got, err := ResolveHost(tt.flag)

			assert.ErrorIs(t, err, tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
