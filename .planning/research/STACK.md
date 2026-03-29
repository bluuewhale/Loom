# Technology Stack — Go GraphRAG Community Detection Library

**Project:** loom (community-detection module)
**Researched:** 2026-03-29
**Overall confidence:** HIGH (core Go tooling) / MEDIUM (library recommendations)

---

## Recommendation: Remain Zero-Dependency

**Verdict: Stay stdlib-only for algorithm code. Add tooling deps only in dev/bench tooling.**

Rationale:

1. **Gonum has Louvain but not Leiden.** `gonum.org/v1/gonum/graph/community` implements
   Louvain via `Modularize`, but has no Leiden implementation (confirmed pkg.go.dev, 2025-12-29
   publish date). Adopting Gonum to get half the algorithms — while still hand-rolling Leiden —
   gives you a dependency without eliminating implementation work.

2. **Gonum's graph interface is heavyweight.** Gonum requires callers to implement
   `graph.Graph`/`graph.WeightedGraph` interfaces. This project already has a leaner, purpose-built
   `Graph` struct. Wrapping it to satisfy Gonum's interface adds boilerplate with zero benefit for
   an isolated algorithm library.

3. **dominikbraun/graph is general-purpose, not performance-oriented.** It uses generics for
   ergonomics (good DX for app code) but is not designed for sub-100ms community detection on
   10 000-node graphs. Benchmarks and profiling support differ from the stdlib path.

4. **Zero-dependency is a library feature.** For a library meant to be embedded in GraphRAG
   pipelines, every transitive dependency is a tax on the caller. The current zero-dep posture is
   a genuine differentiator.

---

## Recommended Stack

### Core — No Changes

| Component | Choice | Version | Rationale |
|-----------|--------|---------|-----------|
| Language | Go | 1.26.1 | Already in use; generics, range-over-func available |
| Graph struct | custom `graph/graph.go` | — | Already weighted, directed/undirected, NodeID-typed |
| Modularity | custom `graph/modularity.go` | — | Already validated against Karate Club |
| Node mapping | custom `graph/registry.go` | — | Optional; hot paths bypass it |

No external runtime dependencies should be added. All algorithm packages (`louvain`, `leiden`,
future centrality) should import only `community-detection/graph` and stdlib.

### Benchmarking Tooling (dev-only, not importable)

| Tool | Source | Purpose | Confidence |
|------|--------|---------|------------|
| `testing.B` | stdlib `testing` | Write `BenchmarkXxx` functions; `-benchmem` flag gives alloc/op | HIGH |
| `pprof` | stdlib `runtime/pprof` + `net/http/pprof` | CPU and heap profiling during benchmarks via `-cpuprofile`/`-memprofile` | HIGH |
| `benchstat` | `golang.org/x/perf/cmd/benchstat` | Statistical comparison of benchmark runs (A/B, CI); install with `go install` | HIGH |
| `go tool pprof` | bundled in Go toolchain | Interactive flamegraph, call-graph exploration; no separate install | HIGH |

`benchstat` is the standard tool for proving a perf regression or improvement with statistical
confidence. Install:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Usage pattern:

```bash
go test -bench=. -benchmem -count=10 ./... > before.txt
# make change
go test -bench=. -benchmem -count=10 ./... > after.txt
benchstat before.txt after.txt
```

### Profiling Workflow (standard Go, no extra deps)

```bash
# CPU profile
go test -bench=BenchmarkLouvain -cpuprofile=cpu.prof ./graph/community/
go tool pprof -http=:8080 cpu.prof

# Memory profile
go test -bench=BenchmarkLouvain -memprofile=mem.prof ./graph/community/
go tool pprof -http=:8080 mem.prof
```

PGO (Profile-Guided Optimization) is available in Go 1.21+ and enabled in 1.26. Once a stable
CPU profile exists, place `default.pgo` in the package directory and `go build` will apply
inlining hints automatically. Relevant for a library shipped as source — callers benefit on their
build.

---

## Alternatives Considered and Rejected

| Category | Rejected Option | Reason |
|----------|----------------|--------|
| Graph library | `gonum.org/v1/gonum/graph` | No Leiden; heavyweight interface; large transitive dep |
| Graph library | `github.com/dominikbraun/graph` | Generic/ergonomic focus, not perf-oriented; no community detection |
| Graph library | `github.com/hmdsefi/gograph` | Less maintained; same interface-mismatch problem |
| Memory allocator | `GOEXPERIMENT=arena` | Experimental, API unstable, proposal on hold indefinitely (golang/go#51317) |
| Test framework | testify | Not needed; stdlib `testing` with table-driven tests suffices for algo validation |

---

## Memory Performance Patterns (no deps required)

For the 10 000-node / <100ms target, use these stdlib-only patterns:

| Pattern | When | Why |
|---------|------|-----|
| Pre-allocate slices with `make([]T, 0, n)` | Community partition arrays, neighbor lists | Avoids growth copies in hot loops |
| `sync.Pool` for temp buffers | Per-pass scratch space in Louvain phase loop | Reduces GC pressure in parallel calls |
| `map[NodeID]int` with `make(map, hint)` | Partition representation | Hint avoids rehashing; matches existing PROJECT.md decision |
| Avoid pointer-heavy structs in hot path | Node adjacency representation | Cache-friendly flat slices outperform pointer graphs |

`sync.Pool` is the correct alternative to the experimental arena package. It is production-stable,
well-understood, and appropriate for the "many small graphs concurrently" use case described in
the project requirements.

---

## What NOT to Use

| Technology | Why Not |
|-----------|---------|
| `gonum.org/v1/gonum/graph/community` (Louvain) | Would require adopting Gonum's graph interface; no Leiden; partial solution adds full dependency weight |
| CGO / C bindings | Violates "no CGO" constraint; breaks cross-compilation |
| `GOEXPERIMENT=arena` | Experimental, unstable API, may be removed; use `sync.Pool` instead |
| Any Python interop (igraph, leidenalg) | Out of scope; library is pure Go |
| `github.com/dominikbraun/graph` | No community detection; wrong abstraction layer |
| External concurrency primitives (errgroup v2, etc.) | `sync.WaitGroup` + `sync.Pool` + channels cover all stated requirements |

---

## Go Version Features Available (1.26.1)

| Feature | Use Case |
|---------|---------|
| Generics (1.18+) | `CommunityDetector` interface can be generic if needed; current `map[NodeID]int` partition is already zero-alloc friendly |
| Range-over-func (1.23+) | Iterator patterns over graph edges without allocating a slice |
| PGO (1.21+, stable in 1.22+) | Build-time optimization from production profiles |
| `slices` / `maps` stdlib packages (1.21+) | Sort partitions, clone maps — no need for `golang.org/x/exp` |

---

## Sources

- [gonum graph/community pkg.go.dev](https://pkg.go.dev/gonum.org/v1/gonum/graph/community) — confirmed Louvain only, no Leiden; published 2025-12-29
- [gonum/graph DEPRECATED repo](https://github.com/gonum/graph) — predecessor, now merged into gonum/gonum
- [benchstat pkg.go.dev](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat) — statistical benchmark comparison
- [golang.org/x/perf/benchstat](https://pkg.go.dev/golang.org/x/perf/benchstat) — library form
- [dominikbraun/graph](https://pkg.go.dev/github.com/dominikbraun/graph) — generic graph, no community detection
- [Go arena proposal on hold](https://github.com/golang/go/issues/51317) — experimental, do not use
- [Leiden algorithm paper](https://www.nature.com/articles/s41598-019-41695-z) — guarantees well-connected communities; Louvain does not
- [Go pprof blog](https://go.dev/blog/pprof) — canonical profiling guide
- [Go arenas 2025 analysis](https://mcyoung.xyz/2025/04/21/go-arenas/) — confirms arena not production-ready
