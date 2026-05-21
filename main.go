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
	ctx := context.Background()
	switch args[0] {
	case "version":
		fmt.Fprintln(os.Stdout, version.Current())
		return exitOK
	case "update":
		return report(run.EnsureLatest(ctx, true))
	case "upgrade":
		return report(run.Upgrade(ctx))
	case "rules":
		err := run.EnsureLatest(ctx, false)
		if err != nil {
			return report(err)
		}
		return report(run.Rules(ctx))
	case "fix", "check":
		err := run.EnsureLatest(ctx, deep)
		if err != nil {
			return report(err)
		}
		return report(run.Gate(ctx, args[0] == "fix", deep))
	default:
		fmt.Fprintln(os.Stdout, usage)
		return exitUsage
	}
}

func report(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go:", err)
		return exitFail
	}
	return exitOK
}
