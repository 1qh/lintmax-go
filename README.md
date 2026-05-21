# lintmax-go

Maximum-strictness Go quality gate. One command, always-latest, never stale.

The Go counterpart to [lintmax](https://github.com/1qh/lintmax) (TypeScript). Designed for coding agents, not humans.

## Why

Go's compiler is already stricter than `tsc` on some axes (unused vars/imports are hard errors). lintmax-go closes the rest of the gap and pushes past it: every linter golangci-lint bundles, maxed internal configs, plus the tools golangci deliberately doesn't wrap — nil-safety, whole-program dead code, vulnerability scanning.

## Never stale

No linter version is ever pinned. Every tool is fetched `@latest` on each run, and the embedded golangci config uses `default: all` — so the moment golangci ships a new linter, your build runs it. You are always on the bleeding edge; you fix-forward, you never accumulate lint debt.

## Install

```
go install github.com/1qh/lintmax-go@latest
```

## Use

```
lintmax-go fix      # format + autofix + fast gate
lintmax-go check    # verify only, no writes (CI)
lintmax-go update   # reinstall every tool @latest
lintmax-go fix --deep / check --deep   # + slow scanners (govulncheck, osv-scanner)
```

Silent on success. Exit 0 = clean.

## What runs

| Layer | Tool | Catches |
| --- | --- | --- |
| format | gofumpt → gci → golines | strict format, deterministic imports, line length |
| lint | golangci-lint `default: all` | ~115 linters, all internal checks maxed |
| nil-safety | nilaway | nil panics (the TS-strict-null gap) |
| dead code | deadcode | whole-program unreachable funcs |
| tests | `go test -race -shuffle=on` | data races, test-order coupling |
| deep | govulncheck + osv-scanner | reachability + lockfile CVEs |

## Strictness policy

- `default: all` — opt out of nothing by default; new linters auto-enabled.
- Only **physical conflicts** disabled (`nlreturn` vs `wsl_v5`), and tools that are structurally per-project (`depguard`) — these belong in a project's own override, not a generic config.
- Maxed internal checks: staticcheck all categories, gocritic all checks, revive all rules, gosec low/low, errcheck type-assertions + blank.
- golangci's silent default exclusions are **off** — nothing is hidden.
- Every suppression requires `//nolint:rule // reason` (nolintlint enforces it).

## License

MIT
