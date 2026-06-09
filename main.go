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
  lintmax sync      write/refresh the standard ci.yml + release.yml + prune scripts into this project
  lintmax cov       run test coverage (go test -coverprofile) + print the total
  lintmax watch     re-run the gate (fix mode) on every .go/.mod/.sum change
  lintmax update    reinstall every linter tool @latest
  lintmax upgrade   reinstall lintmax-go itself @latest
  lintmax version   print lintmax-go's version
  lintmax rules     list every enabled linter under the maxed config
prints "ok" on success, exit 0 = clean; verbose only on failure.`

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
	return dispatch(ctx, args[0])
}

func commands() map[string]func(context.Context) int {
	return map[string]func(context.Context) int{
		"version": func(context.Context) int { fmt.Fprintln(os.Stdout, version.Current()); return exitOK },
		"update":  func(ctx context.Context) int { return reportOK(run.EnsureLatest(ctx, true)) },
		"upgrade": func(ctx context.Context) int { return reportOK(run.Upgrade(ctx)) },
		"init":    func(context.Context) int { return reportOK(run.Init()) },
		"sync":    func(context.Context) int { return reportOK(run.Sync()) },
		"cov":     func(ctx context.Context) int { return reportOK(run.Cov(ctx)) },
		"watch":   func(ctx context.Context) int { return report(run.Watch(ctx)) },
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

func reportOK(err error) int {
	code := report(err)
	if code == exitOK {
		fmt.Fprintln(os.Stdout, "ok")
	}
	return code
}
