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
	"github.com/1qh/lintmax-go/internal/transform"
)

const (
	emptyArg   = ""
	goCmd      = "go"
	configMode = 0o600
)

var (
	ErrGate  = errors.New("gate failed")
	ErrDirty = errors.New("comments/blank-lines present (run fix)")
)

type step struct {
	run  func(context.Context) error
	name string
}

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
func EnsureLatest(ctx context.Context, includeDeep bool) error {
	for _, tool := range tools.All {
		if tool.Deep && !includeDeep {
			continue
		}
		_, _ = fmt.Fprintf(os.Stderr, "• %s@latest (%s)\n", tool.Name, tool.Why)
		cmd := exec.CommandContext(ctx, goCmd, "install", tool.Pkg+"@latest") //nolint:gosec // static registry paths
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("install %s: %w", tool.Name, err)
		}
	}
	return nil
}

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
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed tool invocations
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func transformStep(fix bool) step {
	return step{
		name: "comments+compact",
		run: func(_ context.Context) error {
			if fix {
				_, err := transform.Apply(".")
				if err != nil {
					return fmt.Errorf("transform: %w", err)
				}
				return nil
			}
			changed, err := transform.Check(".")
			if err != nil {
				return fmt.Errorf("transform: %w", err)
			}
			if len(changed) > 0 {
				return fmt.Errorf("%w: %s", ErrDirty, strings.Join(changed, ", "))
			}
			return nil
		},
	}
}

func steps(cfg string, fix, deep bool) []step {
	golangciArgs := []string{"run", "--config", cfg}
	if fix {
		golangciArgs = append(golangciArgs, "--fix")
	}
	out := []step{
		transformStep(fix),
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
