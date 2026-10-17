// Command ollama-tray shows which models an Ollama server has loaded in the system tray.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	// minInterval keeps a typo such as "5ms" from hammering the server.
	minInterval = time.Second
	exitUsage   = 2
)

// errInvalidInterval is returned by parseConfig when --interval is below minInterval.
var errInvalidInterval = errors.New("invalid interval")

type config struct {
	host     string
	interval time.Duration
}

// parseResult is what the command line asked for. When help or version is
// set, cfg is empty and the caller exits without starting the tray.
type parseResult struct {
	cfg     config
	help    bool
	version bool
}

// parseConfig reads the flags and resolves the server address. The flag
// package prints the usage text to stderr for --help and for a flag error.
func parseConfig(args []string, stderr io.Writer) (parseResult, error) {
	fs := flag.NewFlagSet("ollama-tray", flag.ContinueOnError)
	// The flag package would print its own error before the caller prints
	// the returned one, so it prints nothing during Parse.
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "Usage of ollama-tray:")
		fs.SetOutput(stderr)
		fs.PrintDefaults()
	}
	hostFlag := fs.String("host", "", "Ollama server address (default: $OLLAMA_HOST, else http://127.0.0.1:11434)")
	interval := fs.Duration("interval", defaultInterval, "time between status polls (minimum 1s)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return parseResult{help: true}, nil
		}
		return parseResult{}, err
	}
	if *showVersion {
		return parseResult{version: true}, nil
	}
	if *interval < minInterval {
		return parseResult{}, fmt.Errorf("%w: must be at least %s, got %s", errInvalidInterval, minInterval, *interval)
	}
	host, err := ollama.ResolveHost(*hostFlag)
	if err != nil {
		return parseResult{}, err
	}
	return parseResult{cfg: config{host: host, interval: *interval}}, nil
}

func main() {
	res, err := parseConfig(os.Args[1:], os.Stderr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ollama-tray: %v\n", err)
		os.Exit(exitUsage)
	}
	if res.help {
		return
	}
	if res.version {
		fmt.Println(version.String())
		return
	}
	cfg := res.cfg

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := ollama.NewClient(cfg.host, ollama.NewHTTPClient())
	tray.Run(ctx, tray.Config{Client: client, Host: cfg.host, Interval: cfg.interval})
}
