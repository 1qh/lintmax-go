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
  lintmax fix       format + autofix + full gate (every linter + govulncheck + osv-scanner + nilaway + capslock)
  lintmax check     verify only, no writes (CI mode) — same exhaustive scanner set as fix
  lintmax init      scaffold .editorconfig + CI workflow into this project
  lintmax update    reinstall every linter tool @latest
  lintmax upgrade   reinstall lintmax-go itself @latest
  lintmax version   print lintmax-go's version
  lintmax rules     list every enabled linter under the maxed config
silent on success, exit 0 = clean.`

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

func main() { os.Exit(realMain(os.Args[1:])) }

func maybeProfile() func() {
	path := os.Getenv("LINTMAX_CPUPROFILE") //nolint:forbidigo // reason: bootstrap layer owns env reads
	if path == "" {
		return func() {}
	}
	f, err := os.Create(path) //nolint:gosec // user-supplied profile path
	if err != nil {
		return func() {}
	}
	_ = pprof.StartCPUProfile(f) //nolint:errcheck // best-effort profile
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
	switch args[0] {
	case "version":
		fmt.Fprintln(os.Stdout, version.Current())
		return exitOK
	case "update":
		return report(run.EnsureLatest(ctx, true))
	case "upgrade":
		return report(run.Upgrade(ctx))
	case "init":
		return report(run.Init())
	case "rules":
		return rulesCmd(ctx)
	case "fix", "check":
		return gateCmd(ctx, args[0] == "fix")
	default:
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
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
