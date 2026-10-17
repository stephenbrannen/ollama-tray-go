package ollama

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHost = "127.0.0.1"
	defaultPort = "11434"
)

// ErrInvalidPort is returned by ResolveHost when the port is not a number
// from 1 to 65535.
var ErrInvalidPort = errors.New("invalid port")

// ResolveHost returns the server base URL: the flag if set, else OLLAMA_HOST,
// else http://127.0.0.1:11434.
//
// No scheme means http. No port means 11434 for every scheme, including
// https, because an Ollama server is almost never on 80 or 443 and a proxy
// user gives the port explicitly. An empty host means 127.0.0.1, because
// localhost can resolve to ::1 while Ollama listens on IPv4 only. A path
// after the host is kept without its trailing slash. An invalid port is an
// error, not a silent fallback.
func ResolveHost(flag string) (string, error) {
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

	authority, path, _ := strings.Cut(raw, "/")
	path = strings.TrimRight(path, "/")
	if path != "" {
		path = "/" + path
	}

	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		host, port = strings.Trim(authority, "[]"), ""
	}
	if host == "" {
		host = defaultHost
	}
	if port == "" {
		port = defaultPort
	} else if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return "", fmt.Errorf("%w %q", ErrInvalidPort, port)
	}

	return scheme + "://" + net.JoinHostPort(host, port) + path, nil
}
