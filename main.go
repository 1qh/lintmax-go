package main

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/1qh/lintmax-go/internal/run"
	"github.com/1qh/lintmax-go/internal/version"
)

const usage = `lintmax-go — maximum-strictness Go quality gate (always-latest, never stale)
usage:
  lintmax fix       format + autofix + full fast gate
  lintmax check     verify only, no writes (CI mode)
  lintmax init      scaffold .editorconfig + CI workflow into this project
  lintmax update    reinstall every linter tool @latest
  lintmax upgrade   reinstall lintmax-go itself @latest
  lintmax version   print lintmax-go's version
  lintmax rules     list every enabled linter under the maxed config
  lintmax fix --deep / check --deep   add slow scanners (govulncheck, osv-scanner)
silent on success, exit 0 = clean.`

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2
)

func main() { os.Exit(realMain(os.Args[1:])) }

//nolint:nilaway // os.Args is never nil in a running program
func realMain(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
	deep := slices.Contains(args[1:], "--deep")
	inCI := os.Getenv("CI") != ""
	ctx := context.Background()
	switch args[0] {
	case "version":
		fmt.Fprintln(os.Stdout, version.Current())
		return exitOK
	case "update":
		return report(run.EnsureLatest(ctx, true, true))
	case "upgrade":
		return report(run.Upgrade(ctx))
	case "init":
		return report(run.Init())
	case "rules":
		return rulesCmd(ctx, inCI)
	case "fix", "check":
		return gateCmd(ctx, args[0] == "fix", deep, inCI)
	default:
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
}

func rulesCmd(ctx context.Context, inCI bool) int {
	err := run.EnsureLatest(ctx, false, inCI)
	if err != nil {
		return report(err)
	}
	return report(run.Rules(ctx))
}

func gateCmd(ctx context.Context, fix, deep, inCI bool) int {
	err := run.EnsureLatest(ctx, deep, inCI)
	if err != nil {
		return report(err)
	}
	return report(run.Gate(ctx, fix, deep))
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go:", err)
		return exitFail
	}
	return exitOK
}
