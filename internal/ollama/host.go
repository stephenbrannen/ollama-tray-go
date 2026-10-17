package ollama

import (
	"errors"
	"fmt"
	"net"
	"net/url"
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

// ErrInvalidHost is returned by ResolveHost when the address is not a plain
// http or https server address.
var ErrInvalidHost = errors.New("invalid host")

// ResolveHost returns the server base URL: the flag if set, else OLLAMA_HOST,
// else http://127.0.0.1:11434.
//
// No scheme means http. The scheme is case-insensitive and must be http or
// https. No port means 11434 for every scheme, including https, because an
// Ollama server is almost never on 80 or 443 and a proxy user gives the port
// explicitly. An empty host means 127.0.0.1, because localhost can resolve to
// ::1 while Ollama listens on IPv4 only. A path after the host is kept without
// its trailing slash. User info, a query and a fragment are errors: the client
// appends its own paths, so they would send requests somewhere the user did
// not type, and user info would add a Basic auth header. An invalid port is an
// error, not a silent fallback.
func ResolveHost(flag string) (string, error) {
	raw := strings.TrimSpace(flag)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		// net/url exports no sentinel for a bad port, and the existing
		// contract keeps ErrInvalidPort distinct from other bad addresses.
		if strings.Contains(err.Error(), "invalid port") {
			return "", fmt.Errorf("%w in %q", ErrInvalidPort, raw)
		}
		return "", fmt.Errorf("%w %q: %w", ErrInvalidHost, raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w %q: scheme must be http or https", ErrInvalidHost, raw)
	}
	// Checking the raw text also catches an empty query ("box?") that
	// url.URL leaves no trace of.
	if u.User != nil || strings.ContainsAny(raw, "?#") {
		return "", fmt.Errorf("%w %q: user info, query and fragment are not allowed", ErrInvalidHost, raw)
	}

	host, port := u.Hostname(), u.Port()
	if host == "" {
		host = defaultHost
	}
	if port == "" {
		port = defaultPort
	} else if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
		return "", fmt.Errorf("%w %q", ErrInvalidPort, port)
	}

	path := strings.TrimRight(u.EscapedPath(), "/")
	return u.Scheme + "://" + net.JoinHostPort(host, port) + path, nil
}
