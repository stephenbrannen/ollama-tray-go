//go:build task

// Command e2e builds the end-to-end test image, runs the test once, then
// removes the containers. The named volume with the pulled model is kept, so
// later runs skip the download. Extra arguments go to the test binary, for
// example -test.run.
//
// Run it from the repo root with: go run -tags task ./tasks/e2e
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

const composeFile = "e2e/compose.yaml"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(testArgs []string) int {
	args := append([]string{"compose", "-f", composeFile, "run", "--rm", "--build", "tray"}, testArgs...)
	status := docker(args...)
	// Ollama keeps running after `compose run` returns, so it is stopped even when the test fails.
	if downStatus := docker("compose", "-f", composeFile, "down"); status == 0 {
		status = downStatus
	}
	return status
}

// docker runs the docker CLI and returns its exit code.
func docker(args ...string) int {
	// The arguments are this program's constants plus the caller's own test flags.
	cmd := exec.Command("docker", args...) //nolint:gosec
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitCode()
	default:
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
}
