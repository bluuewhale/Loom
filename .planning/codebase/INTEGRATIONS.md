# External Integrations

**Analysis Date:** 2026-04-01

## APIs & External Services

**None.** This codebase has zero external API integrations. All code runs fully in-process with no network calls.

## Data Storage

**Databases:**
- None — all graph data is in-memory at runtime; the `Graph` struct (`graph/graph.go`) uses `map[NodeID][]Edge` and `map[NodeID]float64` backed entirely by Go heap

**File Storage:**
- Test fixtures only: Zachary's Karate Club, Political Books, and College Football datasets are embedded as Go source files in `graph/testdata/karate.go`, `graph/testdata/polbooks.go`, `graph/testdata/football.go` — no file I/O at runtime

**Caching:**
- `sync.Pool` in `graph/louvain_state.go` (`louvainStatePool`) and `graph/leiden_state.go` (`leidenStatePool`) — in-process object pool, not an external cache

## Authentication & Identity

**Auth Provider:**
- Not applicable — pure Go library with no auth requirements or user identity concepts

## Monitoring & Observability

**Error Tracking:**
- None — errors are returned as Go `error` values; no external error reporting

**Logs:**
- None — library code emits no log output; no logging framework imported

**Metrics:**
- None — no Prometheus, OpenTelemetry, or similar instrumentation

## CI/CD & Deployment

**Hosting:**
- Not applicable — pure Go library distributed via Go module proxy (`go get github.com/bluuewhale/loom/graph`)

**CI Pipeline:**
- GitHub Actions: `.github/workflows/go.yml`
  - Trigger: push and PR to `main`
  - Runner: `ubuntu-latest`
  - Steps: `actions/checkout@v4`, `actions/setup-go@v4` (Go 1.26), `go build -v ./...`, `go test -v ./...`
  - Race detector: **not enabled in CI** (CI runs `go test -v ./...` without `-race`); race tests exist locally via `graph/race_test.go`

## Environment Configuration

**Required env vars:**
- None

**Secrets location:**
- Not applicable — no secrets, credentials, or API keys of any kind

## Webhooks & Callbacks

**Incoming:** None

**Outgoing:** None

## External Comparison Tooling (non-library)

These are benchmark comparison utilities only — not part of the library and not imported by library consumers:

**`scripts/go-compare/` (standalone Go binary):**
- Imports `gonum.org/v1/gonum v0.17.0` (`community.Modularize`) as a Louvain reference
- Imports `github.com/ledyba/go-louvain` as a second Louvain reference
- Imports `github.com/vsuryav/leiden-go` to probe for its known infinite-loop bug
- Run manually: `go run scripts/go-compare/` — no CI integration

**`scripts/compare.py` (Python benchmark script):**
- Uses `python-louvain 0.16`, `networkx`, `leidenalg 0.11` (igraph C++ backend)
- Run manually: `python3 scripts/compare.py`
- Reads `bench-baseline.txt` to compute speedup ratios against Go baselines
- No CI integration

## Summary

The main `github.com/bluuewhale/loom` module is a **fully self-contained, offline, zero-dependency Go library**. It uses only the Go standard library. There are no external services, no network calls, no persistent storage, no environment configuration, and no third-party runtime dependencies. The only external tooling is in standalone benchmark comparison scripts (`scripts/`) that are never imported by library consumers.

---

*Integration audit: 2026-04-01*
