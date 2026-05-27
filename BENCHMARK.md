# Benchmark baseline

Local M-series Mac (10-core), eximagent corpus (~30K LOC, 215 tests, heavy deps
incl. clickhouse-go/v2, excelize/v2, genai, otel/sdk).

| Mode | Wall | Notes |
|---|---|---|
| check (warm) | **8.6s** | collect 6.4s + test 8.2s overlap, staleness 0.2s |
| check (cold) | **75s** | go-build + golangci cache wiped |
| check --deep | 2:37 | adds nilaway + govulncheck + osv-scanner + capslock |

## Phases

| Phase | Warm | Cold | Notes |
|---|---|---|---|
| writeConfig | 0s | 0s | embed write |
| transform | 55ms | 55ms | comment strip walk |
| collect | 6.4s | 30s | golangci-lint + deadcode parallel |
| test | 8.2s | 75s | go test -race -shuffle ./... |
| staleness | 200ms | 200ms | GH releases API, parallel |

collect + test overlap via WaitGroup — wall = max, not sum.

## Optimization history

| Step | Warm wall | Why |
|---|---|---|
| baseline (v0.15.1) | 33.6s | sequential collect, nilaway in default |
| + parallel collect (golangci+nilaway+deadcode) | 28.5s | sync.WaitGroup.Go |
| + nilaway → --deep only | 12.7s | nilaway = 18.7s isolated; govet nilness covers baseline |
| + collect+test concurrent | 8.7s | share go-build cache |
| + staleness HTTP parallel | 8.6s | per-action GH API call concurrent |

## Ideas tried + rejected

- **prewarm via `go build -o /dev/null ./...`**: regressed (+33s cold). Race-instrumented test binaries don't share cache with plain build.
- **prewarm via `go test -count=0 -race`**: regressed. golangci-lint's loader cache is separate from go test's race cache.
- **drop `-race` by default**: would save 40s cold but loses concurrency-bug coverage. Not worth the safety regression for a quality gate.

## CI implications

| Runner | Cold | Warm |
|---|---|---|
| local M-series 10c | 75s | 8.6s |
| Blacksmith 4-vcpu ARM | ~3-5min | ~30-60s |
| GH ubuntu-latest 2c | ~6-10min | ~60-120s |

CI hot path: ensure `actions/cache@v5` keys `~/.cache/go-build` +
`~/.cache/golangci-lint` survive go.sum churn (use restore-keys without
trailing dash). Pin `lintmax-go@latest` in consumer workflows; the gate
auto-updates linter deps per 24h TTL.
