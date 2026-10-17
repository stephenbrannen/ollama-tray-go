// Package status maps the result of an Ollama poll to a tray state.
package status

import "github.com/stephenbrannen/ollama-tray-go/internal/ollama"

// Status is what the tray icon shows.
type Status int

const (
	// Unreachable means the server cannot be reached or returned an error.
	Unreachable Status = iota
	// Idle means the server answers but no model is loaded.
	Idle
	// Loaded means at least one model is loaded.
	Loaded
	// Checking means the first poll has not returned yet. The tray starts
	// here so it does not claim the server is down before it has asked.
	Checking
)

// String returns a lowercase name for logs and test failures.
func (s Status) String() string {
	switch s {
	case Loaded:
		return "loaded"
	case Idle:
		return "idle"
	case Checking:
		return "checking"
	case Unreachable:
		fallthrough
	default:
		return "unreachable"
	}
}

// From maps the result of LoadedModels to a Status. Any error counts as
// Unreachable, including an error status or an unreadable reply, because the
// tray then cannot tell which models are loaded.
func From(models []ollama.LoadedModel, err error) Status {
	switch {
	case err != nil:
		return Unreachable
	case len(models) > 0:
		return Loaded
	default:
		return Idle
	}
}
