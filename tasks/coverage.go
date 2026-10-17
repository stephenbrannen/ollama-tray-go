//go:build task

// Command coverage runs the tests with the race detector and fails when
// statement coverage of the logic packages drops below the floor.
//
// Run it with: go run -tags task ./tasks/coverage.go
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// floor is the minimum statement coverage, in percent. Raise it as coverage
// rises; never lower it to make a change pass.
const floor = 80.0

// wiringPackages hold only the systray event loop and process startup. They
// need a display server or a real process, so they stay out of the floor.
var wiringPackages = []string{
	"/cmd/ollama-tray",
	"/internal/tray",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "coverage:", err)
		os.Exit(1)
	}
}

func run() error {
	out, err := exec.Command("go", "list", "./...").Output()
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}
	var logic []string
	for pkg := range strings.FieldsSeq(string(out)) {
		if !isWiring(pkg) {
			logic = append(logic, pkg)
		}
	}

	profile := filepath.Join(os.TempDir(), "ollama-tray-cover-"+strconv.Itoa(os.Getpid())+".out")
	defer func() { _ = os.Remove(profile) }()

	// -coverpkg counts a logic package even when only another package's tests
	// reach it.
	args := []string{
		"test", "-race", "-count=1",
		"-coverpkg=" + strings.Join(logic, ","),
		"-coverprofile=" + profile,
		"./...",
	}
	// The arguments are package paths from go list and a temp file path.
	cmd := exec.Command("go", args...) //nolint:gosec
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go test: %w", err)
	}

	pct, err := percent(profile)
	if err != nil {
		return err
	}
	fmt.Printf("coverage of logic packages: %.1f%% (floor %.1f%%)\n", pct, floor)
	if pct < floor {
		return fmt.Errorf("%.1f%% is below the %.1f%% floor", pct, floor)
	}
	return nil
}

func isWiring(pkg string) bool {
	for _, suffix := range wiringPackages {
		if strings.HasSuffix(pkg, suffix) {
			return true
		}
	}
	return false
}

// percent reads a cover profile. Each line after the mode line is
// "file:start,end statements count".
func percent(path string) (float64, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the temp profile this task wrote
	if err != nil {
		return 0, fmt.Errorf("open profile: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Each test binary writes its own line for every block, with a zero count
	// when it never ran the block. A block is covered if any binary ran it.
	type block struct {
		statements int
		hit        bool
	}
	blocks := map[string]block{}
	sc := bufio.NewScanner(f)
	sc.Scan() // skip the "mode:" line
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 {
			return 0, fmt.Errorf("malformed profile line %q", sc.Text())
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("malformed profile line %q: %w", sc.Text(), err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, fmt.Errorf("malformed profile line %q: %w", sc.Text(), err)
		}
		b := blocks[fields[0]]
		b.statements = statements
		b.hit = b.hit || count > 0
		blocks[fields[0]] = b
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("read profile: %w", err)
	}
	var total, covered int
	for _, b := range blocks {
		total += b.statements
		if b.hit {
			covered += b.statements
		}
	}
	if total == 0 {
		return 0, fmt.Errorf("profile has no statements")
	}
	return 100 * float64(covered) / float64(total), nil
}
