# Benchmark baseline

Local M-series Mac (10-core), eximagent corpus (~30K LOC, 215 tests, heavy deps
incl. clickhouse-go/v2, excelize/v2, genai, otel/sdk).

| Mode | Wall | Notes |
|---|---|---|
| check (re-run no source change) | **14ms** | tree-hash cache hit — `ok (cached)` |
| check --skip-test (lint-only CI lane) | **6.2s** | LINTMAX_SKIP_TEST=1; collect 5s + staleness ~0s |
| check (warm full) | **8-9s** | collect 5-6s + test 7-8s overlap, staleness ~0s |
| check (cold) | **75s** | go-build + golangci cache wiped |
| check --deep | 2:37 | adds nilaway + govulncheck + osv-scanner + capslock |

## Phases (parallel — wall = max not sum)

| Phase | Warm | Cold | Notes |
|---|---|---|---|
| writeConfig | 0s | 0s | embed write |
| treehash | 7ms | 7ms | sha256 over .go/.mod/.sum + version |
| transform | 20ms | 20ms | parallel walk via WaitGroup.Go |
| collect | 5-6s | 30s | golangci-lint (1.5s) + deadcode (4.5s) parallel |
| test | 7-8s | 75s | go test -race -p=4 -shuffle ./... |
| staleness | 0s warm / 1.7s cold | 10s | GH+go.mod cached 24h to disk |

## Optimization history

| Step | Warm wall | Why |
|---|---|---|
| baseline (v0.15.1) | 33.6s | sequential collect, nilaway in default |
| + parallel collect | 28.5s | sync.WaitGroup.Go |
| + nilaway → --deep | 12.7s | nilaway 18.7s isolated; govet nilness covers baseline |
| + collect+test concurrent | 8.7s | share go-build cache |
| + staleness HTTP parallel | 8.6s | per-action GH API call concurrent |
| + go test -p=4 (was default) | 8.0s | race contention hurts above p=4 on M-series |
| + golangci --concurrency=4 | 8.0s | -25% on golangci (1.6→1.2s) |
| + deadcode bug fix | 8.0s | exits 0 even with findings; was silently dropped |
| + tree-hash cache | **14ms** (re-run) | sha256 over source; persist last-green in versions.json |
| + go.mod staleness scan | +1.6s once | go list -m -u -json; cached by go.sum hash |
| + GH releases disk cache (24h) | -1.5s repeat | per-action JSON file in ~/.cache/lintmax-go/staleness |
| + go list disk cache (24h) | -1.6s repeat | keyed by sha256(go.sum) |
| + parallel EnsureLatest | -90s cold CI | 10 tool installs concurrent vs sequential |
| + parallel transform | 53ms→20ms | per-file goroutines |
| + LINTMAX_SKIP_TEST env | 8.6s → 6.2s | consumer CI lane split (don't test twice) |
| + nilaway Deep flag | -10s cold CI | install only when needed |
| + drop force=inCI from EnsureLatest | -30s CI warm | trust 24h TTL + ~/go/bin cache |

## Tree-hash cache

`Gate` first computes `sha256` over every `.go`/`.mod`/`.sum` file plus
`lintmax-go` version, looks it up in `~/Library/Caches/lintmax-go/versions.json`
(keyed by cwd). On hit: skip everything, print `ok (cached)`. On success of a
real run: persist the hash. Invalidated automatically when lintmax-go upgrades
(version is part of the hash).

Bypass with `LINTMAX_NO_SKIP=1`. Disabled for `fix` and `--deep` modes.

## Env knobs

| Env | Default | Effect |
|---|---|---|
| `LINTMAX_TIMING=1` | off | print per-phase wall to stderr |
| `LINTMAX_NO_SKIP=1` | off | bypass tree-hash cache hit |
| `LINTMAX_NO_RACE=1` | off | drop `-race` (saves ~5s warm, 40s cold) |
| `LINTMAX_SKIP_TEST=1` | off | skip test phase entirely |
| `LINTMAX_SKIP_STALENESS=1` | off | skip GH/go.mod staleness scan |
| `LINTMAX_STALENESS_TOLERANCE_DAYS=N` | 7 | min age before flagging stale |
| `LINTMAX_CPUPROFILE=path` | off | write CPU profile to path |
| `GITHUB_TOKEN` | unset | authenticate GH releases API (5000/hr vs 60/hr) |

## Ideas tried + rejected

- **prewarm via `go build -o /dev/null ./...`**: regressed (+33s cold). Race-instrumented test binaries don't share cache with plain build.
- **prewarm via `go test -count=0 -race`**: regressed. golangci-lint's loader cache is separate from go test's race cache.
- **drop `-race` by default**: would save 40s cold but loses concurrency-bug coverage. Available via `LINTMAX_NO_RACE=1` opt-in.
- **drop deadcode (overlap with `unused`)**: deadcode catches whole-program unreachability that staticcheck `unused` misses (per-package). Real finding on eximagent (OpenClickHouse unreachable). Keep.
- **in-process golangci-lint library**: ~50ms subprocess save vs huge code-import change. Skip.
- **per-phase tree hashing**: complex; tree-hash already covers "all unchanged" case. Skip.

## CI implications

| Runner | Cold | Warm |
|---|---|---|
| local M-series 10c | 75s | 8-9s |
| Blacksmith 4-vcpu ARM | ~3-5min | ~30-60s |
| GH ubuntu-latest 2c | ~6-10min | ~60-120s |

CI hot path: ensure `actions/cache@v5` keys `~/.cache/go-build` +
`~/.cache/golangci-lint` + `~/.cache/lintmax-go` + `~/go/bin` survive go.sum
churn (use restore-keys without trailing dash). Pin `lintmax-go@latest` in
consumer workflows; the gate auto-updates linter deps per 24h TTL.

For lint+test split (separate jobs): set `LINTMAX_SKIP_TEST=1` in lint job to
avoid running tests twice (once inside `lintmax-go check`, once in the test
job). Saves the entire test phase wall in the lint lane.
