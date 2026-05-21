// Package run orchestrates the tools: install @latest, then format/lint/check.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1qh/lintmax-go/internal/config"
	"github.com/1qh/lintmax-go/internal/tools"
)

const (
	emptyArg   = ""
	goCmd      = "go"
	configMode = 0o600
)

// ErrGate reports that one or more gate steps failed; names are appended.
var ErrGate = errors.New("gate failed")

// step is one named stage of the gate.
type step struct {
	run  func(context.Context) error
	name string
}

// binDir is where `go install` drops binaries.
func binDir() string {
	if v := os.Getenv("GOBIN"); v != emptyArg {
		return v
	}

	if v := os.Getenv("GOPATH"); v != emptyArg {
		return filepath.Join(v, "bin")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = emptyArg
	}

	return filepath.Join(home, goCmd, "bin")
}

func bin(name string) string { return filepath.Join(binDir(), name) }

// EnsureLatest installs every tool @latest. Always-latest = never stale.
//
//nolint:revive // includeDeep is a genuine install-scope mode, not control coupling
func EnsureLatest(ctx context.Context, includeDeep bool) error {
	for _, tool := range tools.All {
		if tool.Deep && !includeDeep {
			continue
		}

		_, _ = fmt.Fprintf(os.Stderr, "• %s@latest (%s)\n", tool.Name, tool.Why)
		// gosec G204: package paths come from our own static registry, not user input.
		cmd := exec.CommandContext(ctx, goCmd, "install", tool.Pkg+"@latest") //nolint:gosec // static registry paths
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr

		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("install %s: %w", tool.Name, err)
		}
	}

	return nil
}

// writeConfig drops the embedded maxed config to a temp path golangci reads.
func writeConfig() (string, error) {
	dir, err := os.MkdirTemp(emptyArg, "lintmax-go")
	if err != nil {
		return emptyArg, fmt.Errorf("temp dir: %w", err)
	}

	path := filepath.Join(dir, ".golangci.yml")

	err = os.WriteFile(path, config.GolangCI, configMode)
	if err != nil {
		return emptyArg, fmt.Errorf("write config: %w", err)
	}

	return path, nil
}

func sh(ctx context.Context, name string, args ...string) error {
	// gosec G204: name/args are our own fixed tool invocations, not user input.
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed tool invocations
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}

	return nil
}

//nolint:revive // fix/deep are genuine gate modes, not control coupling
func steps(cfg string, fix, deep bool) []step {
	golangciArgs := []string{"run", "--config", cfg}
	if fix {
		golangciArgs = append(golangciArgs, "--fix")
	}

	out := []step{
		{
			name: "golangci-lint",
			run:  func(c context.Context) error { return sh(c, bin("golangci-lint"), golangciArgs...) },
		},
		{name: "nilaway", run: func(c context.Context) error { return sh(c, bin("nilaway"), "./...") }},
		{name: "deadcode", run: func(c context.Context) error { return sh(c, bin("deadcode"), "-test", "./...") }},
		{
			name: "go test -race",
			run:  func(c context.Context) error { return sh(c, goCmd, "test", "-race", "-shuffle=on", "./...") },
		},
	}
	if deep {
		out = append(
			out,
			step{name: "govulncheck", run: func(c context.Context) error { return sh(c, bin("govulncheck"), "./...") }},
			step{
				name: "osv-scanner",
				run:  func(c context.Context) error { return sh(c, bin("osv-scanner"), "scan", "source", "-r", ".") },
			},
		)
	}

	return out
}

// Gate runs the full strictness gate. fix=true autofixes; deep adds slow scanners.
func Gate(ctx context.Context, fix, deep bool) error {
	cfg, err := writeConfig()
	if err != nil {
		return err
	}

	var failed []string

	for _, s := range steps(cfg, fix, deep) {
		runErr := s.run(ctx)
		if runErr != nil {
			failed = append(failed, s.name)
		}
	}

	if len(failed) > 0 {
		return fmt.Errorf("%w: %s", ErrGate, strings.Join(failed, ", "))
	}

	return nil
}
