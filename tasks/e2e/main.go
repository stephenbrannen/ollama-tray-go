//go:build task

// Command e2e builds the end-to-end test image, runs the test once, then
// removes the containers. The named volume with the pulled model is kept, so
// later runs skip the download. Extra arguments go to the test binary, for
// example -test.run.
//
// Run it from the repo root with: go run -tags task ./tasks/e2e
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

const composeFile = "e2e/compose.yaml"

func main() {
	// Ctrl-C cancels the context, which stops the docker client.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	status := run(ctx, os.Args[1:])
	stop()
	os.Exit(status)
}

func run(ctx context.Context, testArgs []string) int {
	args := append([]string{"compose", "-f", composeFile, "run", "--rm", "--build", "tray"}, testArgs...)
	status := docker(ctx, args...)
	// Ollama keeps running after `compose run` returns, so it is stopped even
	// when the test fails or Ctrl-C cancelled ctx. A cancelled ctx would stop
	// the down command at once, so it gets a context of its own.
	if downStatus := docker(context.WithoutCancel(ctx), "compose", "-f", composeFile, "down"); status == 0 {
		status = downStatus
	}
	return status
}

// docker runs the docker CLI and returns its exit code.
func docker(ctx context.Context, args ...string) int {
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // the arguments are this program's constants plus the caller's own test flags
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
