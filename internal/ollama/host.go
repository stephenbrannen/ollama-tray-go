package ollama

import (
	"net"
	"os"
	"strings"
)

const (
	defaultHost      = "localhost"
	defaultPort      = "11434"
	defaultHTTPSPort = "443"
)

// ResolveHost returns the server base URL: the flag if set, else OLLAMA_HOST,
// else http://localhost:11434.
//
// The scheme and port defaults match the ollama CLI, so a value that works
// for `ollama ps` works here: no scheme means http, and no port means 11434
// (443 for https).
func ResolveHost(flag string) string {
	raw := strings.TrimSpace(flag)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}

	scheme := "http"
	if rest, ok := strings.CutPrefix(raw, "http://"); ok {
		raw = rest
	} else if rest, ok := strings.CutPrefix(raw, "https://"); ok {
		scheme = "https"
		raw = rest
	}

	raw = strings.TrimRight(raw, "/")
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host, port = strings.Trim(raw, "[]"), ""
	}
	if host == "" {
		host = defaultHost
	}
	if port == "" {
		port = defaultPort
		if scheme == "https" {
			port = defaultHTTPSPort
		}
	}

	return scheme + "://" + net.JoinHostPort(host, port)
}
