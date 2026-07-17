package main

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"

	"github.com/1qh/lintmax-go/internal/run"
	"github.com/1qh/lintmax-go/internal/version"
)

const usage = `lintmax-go — maximum-strictness Go quality gate (always-latest, never stale, all-scanners-always)
usage:
  lintmax fix       format + autofix + full gate (every linter + govulncheck + osv-scanner + nilaway + capslock); default
  lintmax check     verify only, no writes (CI mode) — same exhaustive scanner set as fix
  lintmax version   print lintmax-go's version
  lintmax rules     list every enabled linter under the maxed config
prints "ok" on success, exit 0 = clean; verbose only on failure.
self-evolving (automatic, never a command): linter/self @latest refresh, CI+release-workflow currency,
green-tree-hash cache, staleness scan — all run internally on every gate.`

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
	cmdFix    = "fix"
)

func main() { os.Exit(realMain(os.Args[1:])) }

func maybeProfile() func() {
	path := os.Getenv("LINTMAX_CPUPROFILE")
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path) //nolint:gosec // user-supplied profile path
	if err != nil {
		fmt.Fprintf(os.Stderr, "lintmax: cannot write LINTMAX_CPUPROFILE=%s: %v\n", path, err)
		return func() {}
	}
	startErr := pprof.StartCPUProfile(f)
	if startErr != nil {
		fmt.Fprintf(os.Stderr, "lintmax: cannot start CPU profile: %v\n", startErr)
		_ = f.Close() //nolint:errcheck // close on a path that already failed
		return func() {}
	}
	return func() {
		pprof.StopCPUProfile()
		_ = f.Close() //nolint:errcheck // close on shutdown non-actionable
	}
}

//nolint:nilaway // os.Args is never nil in a running program
func realMain(args []string) int {
	stop := maybeProfile()
	defer stop()
	if len(args) == 0 {
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
	ctx := context.Background() //nolint:forbidigo // reason: bootstrap top-level ctx seed
	return dispatch(ctx, args[0])
}

func commands() map[string]func(context.Context) int {
	return map[string]func(context.Context) int{
		"version": func(context.Context) int { fmt.Fprintln(os.Stdout, version.Current()); return exitOK },
		"rules":   rulesCmd,
		cmdFix:    func(ctx context.Context) int { return gateCmd(ctx, true) },
		"check":   func(ctx context.Context) int { return gateCmd(ctx, false) },
	}
}

func dispatch(ctx context.Context, cmd string) int {
	handler, ok := commands()[cmd]
	if !ok {
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
	return handler(ctx)
}

func rulesCmd(ctx context.Context) int {
	err := run.EnsureLatest(ctx, false)
	if err != nil {
		return report(err)
	}
	return report(run.Rules(ctx))
}

func gateCmd(ctx context.Context, fix bool) int {
	err := run.EnsureLatest(ctx, false)
	if err != nil {
		return report(err)
	}
	return report(run.Gate(ctx, fix))
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go:", err)
		return exitFail
	}
	return exitOK
}
