# lintmax-go

Maximum-strictness Go quality gate. One command, always-latest, never stale.

The Go counterpart to [lintmax](https://github.com/1qh/lintmax) (TypeScript). Designed for coding agents, not humans.

## Why

Go's compiler is already stricter than `tsc` on some axes (unused vars/imports are hard errors). lintmax-go closes the rest of the gap and pushes past it: every linter golangci-lint bundles, maxed internal configs, plus the tools golangci deliberately doesn't wrap — nil-safety, whole-program dead code, vulnerability scanning.

## Never stale

No linter version is ever pinned. Every tool is fetched `@latest` on each run, and the embedded golangci config uses `default: all` — so the moment golangci ships a new linter, your build runs it. You are always on the bleeding edge; you fix-forward, you never accumulate lint debt.

This extends to your own dependencies. A realtime staleness scan checks `go.mod` deps and GitHub-Action pins against upstream the instant a newer release publishes (tolerance 0; opt into a buffer via `LINTMAX_STALENESS_TOLERANCE_DAYS`). A stale **direct** dep — or a stale **used indirect** one — hard-fails the gate; graph-only modules the build never compiles are ignored. Indirect deps that are MVS-capped (a transitive can't exceed what its requiring module pins) are reported as a non-blocking advisory rather than failing on the unfixable. Set `LINTMAX_SKIP_STALENESS=1` to disable.

## Install

```
go install github.com/1qh/lintmax-go@latest   # Go users

brew tap 1qh/tap                              # macOS / Linux
brew install lintmax-go

# or prebuilt binary (no Go, no brew):
curl -fsSL https://raw.githubusercontent.com/1qh/lintmax-go/main/install.sh | sh
```

Then `lintmax-go init` drops a `.editorconfig` (tab_width=2) + CI workflow into your project.

## Use

```
lintmax-go fix      # format + autofix + fast gate
lintmax-go check    # verify only, no writes (CI)
lintmax-go init     # scaffold .editorconfig + CI workflow
lintmax-go update   # reinstall every linter tool @latest
lintmax-go upgrade  # reinstall lintmax-go itself @latest
lintmax-go version  # print version
lintmax-go rules    # list every enabled linter under the maxed config
lintmax-go fix --deep / check --deep   # + slow scanners (govulncheck, osv-scanner, capslock)
```

Prints `ok` on a single line on success, exit 0 = clean. Tool output is shown only on failure (verbose). The `ok` is the explicit success signal an agent needs to confirm the gate ran and passed; a clean run that is cached prints `ok (cached)`.

## What runs

| Layer | Tool | Catches |
| --- | --- | --- |
| comments + compact | native (go/scanner + go/ast) | deletes all comments except directives; removes blank lines inside function bodies |
| format | gofumpt → gci → golines | strict format, deterministic imports, line length 123 |
| lint | golangci-lint `default: all` | ~115 linters, all internal checks maxed, error-or-off |
| nil-safety | nilaway | nil panics (the TS-strict-null gap) |
| dead code | deadcode | whole-program unreachable funcs |
| tests | `go test -race -shuffle=on` | data races, test-order coupling |
| deep | govulncheck + osv-scanner | reachability + lockfile CVEs |

## Parity with lintmax

Mirrors lintmax (TypeScript): every rule **error or off** (never warn), `default: all` (auto-inherits new linters), comment deletion + compaction, 2-wide indent (`.editorconfig tab_width=2`), line width 123.

Two hard limits Go's gofmt imposes that lintmax doesn't hit (cannot be overridden without abandoning gofmt):

- **Tabs, not spaces** — gofmt mandates tabs; `tab_width=2` makes them render identically to lintmax's 2-space.
- **Top-level blank lines** — gofmt forces a blank after `package`, around imports, and between top-level declarations. Blank lines *inside* function bodies are removed (true "no empty lines between statements"); the top-level ones are gofmt law.

## Earned disables

The disable list starts empty. Each entry is **earned** by a concrete conflict found on real code, never anticipated — feature-forced (comment-strip/compact removing what a rule demands) or lintmax-parity (matching lintmax's own OFF-list). See `internal/config/golangci.yml` for the documented reason on each.

## Project overrides

The bundled config is **generic only** — it carries no project/ecosystem opinion (import paths, in-house ban patterns, first-party struct scope). A consumer supplies those from a `.lintmax-override.yml` in its repo root; lintmax-go merges it into the generated config at run time:

```yaml
exhaustructInclude:        # enables exhaustruct, scoped to YOUR first-party packages
  - '^github\.com/you/yourapp.*'
forbidigoForbid:           # YOUR call-site bans (e.g. env/clock single-source policy)
  - pattern: '^os\.Getenv$'
    msg: "env vars flow through yourapp.Config"
  - pattern: '^time\.Now$'
    msg: "time flows through yourapp.Clock"
```

`exhaustruct` ships disabled (with no include it would flag every third-party `T{}`), and `forbidigo` ships generic stdlib-discipline bans only (`log.*`, bare `fmt.Print*`, `context.Background` in tests). Without an override file these stay generic; the override never lowers strictness, only adds project scope.

## Strictness policy

- `default: all` — opt out of nothing by default; new linters auto-enabled.
- Only **physical conflicts** disabled (`nlreturn` vs `wsl_v5`), and tools that are structurally per-project (`depguard`) — these belong in a project's own override, not a generic config.
- Maxed internal checks: staticcheck all categories, gocritic all checks, revive all rules, gosec low/low, errcheck type-assertions + blank.
- golangci's silent default exclusions are **off** — nothing is hidden.
- Every suppression requires `//nolint:rule // reason` (nolintlint enforces it).

## License

MIT
