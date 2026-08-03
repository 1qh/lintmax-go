# Code-quality policy

The strictness contract lintmax-go enforces, and the rules that govern it.

## Principles

1. **Maximum from line one.** `default: all` — every linter golangci-lint bundles is on, plus the standalone tools it doesn't wrap (nilaway, deadcode, govulncheck, osv-scanner, capslock). Internal checks of each linter are pushed to their maximum.
2. **Never stale.** No version is pinned. Every tool is fetched `@latest` each run; `default: all` auto-enables new linters the day golangci ships them. You ride the bleeding edge and fix-forward.
3. **Error or off, never warn.** Every finding fails the gate. There is no warning tier (parity with lintmax `warnToError`).
4. **Fix the code, don't suppress.** When a rule fires, the first move is to satisfy it. Suppression (`//nolint:rule // reason`) is for genuinely unavoidable, isolated cases and always carries a reason — `nolintlint` enforces that.
5. **Disable nothing up front.** The disable list starts empty. Each entry is _earned_ by a concrete conflict found on real code, and documented with its reason in `internal/config/golangci.yml`.

## One exhaustive gate

`lintmax-go fix` (the default) and `lintmax-go check` (CI, read-only) run the identical, complete scanner set every time — nothing is held back for a separate tier:

comments+compact, golangci (all), nilaway, deadcode, `go test -race -shuffle`, govulncheck, osv-scanner, capslock, plus the in-house dupconst / floatdiv / idiom analyzers and the staleness scan. A clean `check` is skipped via the green-tree-hash cache when nothing changed (`ok (cached)`); `fix` always runs in full. Nothing escapes the merge gate.

## How a disable gets earned

1. A real consumer hits a rule that truly kills productivity, or a rule structurally conflicts with a core feature (comment-strip / compact) or with gofmt.
2. The single rule is added to `disable:` (or a revive rule set `disabled: true`) with a one-line reason.
3. lintmax-go is released; consumers inherit it via `@latest`.

The current earned set falls in two buckets:

- **Feature-forced** — comment-strip + compact remove what a rule demands: `wsl`, `wsl_v5`, `nlreturn` (blank lines), revive `exported` / `package-comments` + staticcheck `ST1000` (doc/package comments), never-fail writers (`fmt.Fprint*`, `Builder.Write`).
- **lintmax-parity / no-equivalent** — `mnd` + revive `add-constant` (`no-magic-numbers: off`), `gochecknoglobals` (lintmax allows module decls), revive `cognitive-complexity` / `flag-parameter`, `testpackage`, `depguard` (non-functional without project config), `gomodguard` (deprecated).

## Hard gofmt limits (cannot be overridden)

- **Tabs, not spaces.** gofmt mandates tabs; `.editorconfig tab_width=2` makes them render like lintmax's 2-space.
- **Top-level blank lines.** gofmt forces blanks after `package`, around imports, and between top-level declarations. lintmax-go removes blank lines _inside_ function bodies (true "no empty lines between statements"); the top-level ones are gofmt law.
