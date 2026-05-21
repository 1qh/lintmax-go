package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/1qh/lintmax-go/internal/config"
	"github.com/1qh/lintmax-go/internal/state"
	"github.com/1qh/lintmax-go/internal/tools"
	"github.com/1qh/lintmax-go/internal/transform"
	"github.com/1qh/lintmax-go/internal/version"
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
func toolVersion(ctx context.Context, binPath string) string {
	out, err := exec.CommandContext(ctx, goCmd, "version", "-m", binPath).Output() //nolint:gosec // fixed binary path
	if err != nil {
		return emptyArg
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" {
			return fields[2]
		}
	}
	return emptyArg
}

func reportBumps(ctx context.Context, installed []tools.Tool) {
	prev := state.Load()
	next := state.State{Versions: map[string]string{}}
	for _, tool := range installed {
		ver := toolVersion(ctx, bin(tool.Name))
		next.Versions[tool.Name] = ver
		old := prev.Versions[tool.Name]
		if old != emptyArg && old != ver {
			fmt.Fprintf(os.Stderr, "↑ %s %s → %s\n", tool.Name, old, ver)
		}
	}
	saveErr := next.Save()
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go: version cache:", saveErr)
	}
}

func EnsureLatest(ctx context.Context, includeDeep bool) error {
	var installed []tools.Tool
	for _, tool := range tools.All {
		if tool.Deep && !includeDeep {
			continue
		}
		cmd := exec.CommandContext(ctx, goCmd, "install", tool.Pkg+"@latest") //nolint:gosec // static registry paths
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		if err != nil {
			fmt.Fprint(os.Stderr, buf.String())
			return fmt.Errorf("install %s: %w", tool.Name, err)
		}
		installed = append(installed, tool)
	}
	reportBumps(ctx, installed)
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
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		fmt.Fprint(os.Stderr, buf.String())
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func Upgrade(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, goCmd, "install", version.Self+"@latest") //nolint:gosec // fixed self path
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		fmt.Fprint(os.Stderr, buf.String())
		return fmt.Errorf("upgrade: %w", err)
	}
	return nil
}

func Rules(ctx context.Context) error {
	cfg, err := writeConfig()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin("golangci-lint"), "linters", "--config", cfg) //nolint:gosec // fixed invocation
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	runErr := cmd.Run()
	if runErr != nil {
		return fmt.Errorf("rules: %w", runErr)
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
			step{
				name: "capslock",
				run:  func(c context.Context) error { return sh(c, bin("capslock"), "-packages", "./...") },
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
