// Command ollama-tray shows which models an Ollama server has loaded in the system tray.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stephenbrannen/ollama-tray-go/internal/ollama"
	"github.com/stephenbrannen/ollama-tray-go/internal/tray"
	"github.com/stephenbrannen/ollama-tray-go/internal/version"
)

const (
	defaultInterval = 5 * time.Second
	// requestTimeout is on the client because ollama.Client leaves timeouts to its caller.
	requestTimeout = 2 * time.Second
	exitUsage      = 2
)

// errReported marks a flag error the flag package already printed.
var errReported = errors.New("flag error already reported")

// errVersion reports that --version was requested; the caller prints the version and exits.
var errVersion = errors.New("version requested")

type config struct {
	host     string
	interval time.Duration
}

// parseConfig reads the flags and resolves the server address.
func parseConfig(args []string, stderr io.Writer) (config, error) {
	fs := flag.NewFlagSet("ollama-tray", flag.ContinueOnError)
	fs.SetOutput(stderr)
	hostFlag := fs.String("host", "", "Ollama server address (default: $OLLAMA_HOST, else http://127.0.0.1:11434)")
	interval := fs.Duration("interval", defaultInterval, "time between status polls")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config{}, err
		}
		return config{}, errReported
	}
	if *showVersion {
		return config{}, errVersion
	}
	if *interval <= 0 {
		return config{}, fmt.Errorf("--interval must be positive, got %s", *interval)
	}
	host, err := ollama.ResolveHost(*hostFlag)
	if err != nil {
		return config{}, err
	}
	return config{host: host, interval: *interval}, nil
}

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	switch {
	case errors.Is(err, flag.ErrHelp):
		return
	case errors.Is(err, errVersion):
		fmt.Println(version.String())
		return
	case err != nil:
		if !errors.Is(err, errReported) {
			fmt.Fprintf(os.Stderr, "ollama-tray: %v\n", err)
		}
		os.Exit(exitUsage)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := ollama.NewClient(cfg.host, &http.Client{Timeout: requestTimeout})
	tray.Run(ctx, tray.Config{Client: client, Host: cfg.host, Interval: cfg.interval})
}
