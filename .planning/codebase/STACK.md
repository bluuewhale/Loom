# Technology Stack

**Analysis Date:** 2026-04-01

## Languages

**Primary:**
- Go 1.26 — all library and algorithm code in `graph/`; declared in `go.mod` and enforced in CI

**Secondary:**
- Python 3 — benchmark comparison script only (`scripts/compare.py`); not part of the library

## Runtime

**Environment:**
- Go 1.26 (set in `go.mod`, enforced in `.github/workflows/go.yml` via `actions/setup-go@v4 go-version: '1.26'`)
- Minimum functional version: Go 1.21 (uses `slices` package first added in 1.21)

**Package Manager:**
- Go modules (`go mod`)
- Main module (`github.com/bluuewhale/loom`): **no `go.sum`** — zero external dependencies
- Comparison script module (`scripts/go-compare/`): has `go.sum` for its external deps

## Frameworks

**Core:**
- Pure Go stdlib — zero external dependencies in the main module
- No CGo, no third-party packages in any `graph/*.go` file

**Testing:**
- `testing` (stdlib) — unit tests, benchmarks, race-condition tests
- No testify, gomock, or other test helpers

**Build/Dev:**
- `go build` / `go test` — standard toolchain; no Makefile, no task runner
- GitHub Actions CI: `.github/workflows/go.yml` runs `go build -v ./...` then `go test -v ./...` on `ubuntu-latest`

## Key Dependencies

**Main module (`github.com/bluuewhale/loom`):**
- **None** — intentionally zero external dependencies

**Comparison/benchmark module only (`scripts/go-compare/go.mod`):**
- `gonum.org/v1/gonum v0.17.0` — used only in benchmark comparison binary; not in the library
- `github.com/ledyba/go-louvain v0.0.0-20220113123819-4f03491a0437` — comparison baseline
- `github.com/vsuryav/leiden-go v0.0.0-20251120005855-0f56599dc139` — comparison baseline (has known infinite-loop bug on large graphs, as noted in `scripts/go-compare/main.go`)

**Python comparison (`scripts/compare.py`) — not Go deps:**
- `networkx` — graph generation
- `python-louvain 0.16` — Louvain baseline
- `leidenalg 0.11` + `igraph 1.0` C++ backend — Leiden baseline

## Standard Library Packages Used in Library Code

| Package | File(s) | Purpose |
|---------|---------|---------|
| `errors` | `graph/detector.go`, `graph/ego_splitting.go` | Sentinel error values (`ErrDirectedNotSupported`, `ErrEmptyGraph`) |
| `math` | `graph/louvain.go`, `graph/leiden.go` | `math.Inf(-1)` for best-Q tracking |
| `math/rand` | `graph/louvain_state.go`, `graph/leiden_state.go` | Per-run RNG for node shuffle; `rand.New(rand.NewSource(seed))` |
| `runtime` | `graph/ego_splitting.go` | `runtime.GOMAXPROCS(0)` to size goroutine worker pool |
| `slices` | `graph/louvain.go`, `graph/louvain_state.go`, `graph/leiden.go`, `graph/leiden_state.go` | `slices.Sort` for deterministic node ordering; `slices.Sort` on candidate list |
| `sort` | `graph/omega.go` | `sort.Slice` for Omega index pair sorting |
| `sync` | `graph/louvain_state.go`, `graph/leiden_state.go`, `graph/ego_splitting.go` | `sync.Pool` for state reuse; `sync.WaitGroup` for goroutine pool |
| `time` | `graph/louvain_state.go`, `graph/leiden_state.go`, `graph/leiden.go` | `time.Now().UnixNano()` for non-deterministic seed when `Seed==0` |

## Configuration

**Environment:**
- No environment variables required
- No config files (no `.env`, YAML, TOML, JSON configs)
- All algorithm behavior is configured via option structs passed at call time: `LouvainOptions`, `LeidenOptions`, `EgoSplittingOptions`

**Build:**
- `go.mod` root: module `github.com/bluuewhale/loom`, `go 1.26`
- No build tags; no `//go:build` constraints beyond test-file naming conventions (`race_test.go`, `norace_test.go`)

## Platform Requirements

**Development:**
- Go 1.21+ (minimum for `slices` package); `go.mod` declares 1.26
- No OS or architecture constraints
- Benchmarks measured on Apple M4 arm64 (per `bench-baseline.txt`)

**Production:**
- Pure Go library: `go get github.com/bluuewhale/loom/graph`
- No binary output; no deployment artifact

## Performance Characteristics (from `bench-baseline.txt`, Apple M4 arm64)

| Benchmark | Time/op | Allocs/op | Memory/op |
|-----------|---------|-----------|-----------|
| `BenchmarkLouvain1K` | ~5.2–5.6 ms | 5,217 | ~2.3 MB |
| `BenchmarkLeiden1K` | ~5.4–6.0 ms | 7,248 | ~2.6 MB |
| `BenchmarkLouvain10K` | ~59–92 ms | 48,773–48,808 | ~18.6 MB |
| `BenchmarkLeiden10K` | ~63–89 ms | 66,512–66,543 | ~21.0 MB |
| `BenchmarkLouvainWarmStart` (10K, ~1% perturbation) | ~42–55 ms | 25,754 | ~10.3 MB |
| `BenchmarkLeidenWarmStart` (10K, ~1% perturbation) | ~48–55 ms | 37,121–37,141 | ~13.2 MB |
| `BenchmarkComputeModularityKarate` | ~1.9–4.2 µs | 3 | 320 B |

**Key optimizations already in place:**
- `sync.Pool` for `louvainState` and `leidenState` — eliminates per-call GC pressure (`graph/louvain_state.go`, `graph/leiden_state.go`)
- Dirty-list (`neighborDirty []NodeID`) — avoids full `clear(neighborBuf)` in the hot loop of `phase1` (`graph/louvain.go`)
- Pre-allocated slice buffers `candidateBuf`, `neighborDirty` with initial capacity 64 — avoids frequent growth
- Insertion sort on candidate list in `phase1` — O(d) where d is node degree, small in practice
- `slices.Sort` (introsort) for deterministic node ordering before shuffle
- Parallel goroutine pool (`runtime.GOMAXPROCS(0)` workers) for ego-net detection in `graph/ego_splitting.go`
- Warm-start via `InitialPartition` option — ~1.4–1.5x speedup on 1% perturbations (enforced by `TestLouvainWarmStartSpeedup`, `TestLeidenWarmStartSpeedup`)
- Incremental persona graph patching in `buildPersonaGraphIncremental` — only re-wires edges for affected nodes
- `CommStrength` cached in `commStr map[int]float64` — avoids re-summing per ΔQ computation

**Remaining allocation hotspots visible in benchmarks:**
- Louvain 10K: ~48,773 allocs/op, ~18.6 MB — primarily from `buildSupergraph` map allocations and `reconstructPartition` copy per phase
- Leiden 10K: ~66,524 allocs/op, ~21 MB — adds `refinePartition` BFS allocations on top of Louvain cost
- `normalizePartition` creates a fresh `map[NodeID]int` and `map[int]int` each call — called once per outer loop iteration
- `buildSupergraph` creates `map[edgeKey]float64` and `map[NodeID]float64` (selfLoops, interEdges) without pooling
- `refinePartition` allocates `inComm map[NodeID]struct{}` and `visited map[NodeID]bool` per community — not pooled
- `reconstructPartition` allocates a new `map[NodeID]int` per candidate-Q evaluation inside the outer loop in addition to the final one

---

*Stack analysis: 2026-04-01*
