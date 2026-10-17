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
)

// String returns a lowercase name for logs and test failures.
func (s Status) String() string {
	switch s {
	case Loaded:
		return "loaded"
	case Idle:
		return "idle"
	case Unreachable:
		fallthrough
	default:
		return "unreachable"
	}
}

// From maps the result of LoadedModels to a Status. Any error counts as
// Unreachable, because a server that answers with an error cannot be asked to
// unload anything either.
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
