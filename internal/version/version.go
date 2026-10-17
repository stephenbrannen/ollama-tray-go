// Package version reports the build version of ollama-tray.
package version

// Version is overridden at build time with -ldflags "-X .../internal/version.Version=1.2.3".
// GoReleaser passes its version without the leading "v".
var Version = "dev"

// String returns the human-readable program name and version.
func String() string {
	return "ollama-tray " + Version
}
