# Project Research Summary

**Project:** loom — Go GraphRAG community-detection library
**Domain:** High-performance graph community detection (Louvain + Leiden) for GraphRAG pipelines
**Researched:** 2026-03-29
**Confidence:** HIGH

---

## Executive Summary

loom is a zero-dependency Go library delivering Louvain and Leiden community detection for real-time GraphRAG pipelines. The existing codebase already has the correct foundations: a weighted directed/undirected `Graph`, validated `ComputeModularity`, and `NodeRegistry`. The research verdict is to build directly on these with no external graph library — Gonum covers Louvain but not Leiden, requires a heavyweight interface wrapper, and would add transitive dependency weight with no net reduction in implementation effort. Pure stdlib with `sync.Pool` and flat-slice data structures meets the 10K-node / <100ms performance target.

The recommended algorithm priority is Leiden first. Louvain produces disconnected communities — Traag et al. found up to 25% badly connected and 16% fully disconnected in practice. For GraphRAG, a disconnected community conflates semantically unrelated entities and degrades summarisation quality. Leiden's refinement phase guarantees community connectivity at a cost that is actually faster than Louvain in benchmarks (20–150% speedup). Louvain should still be implemented as a swap-in alternative for callers with existing pipelines.

The dominant risks are all algorithm-correctness traps, not architecture uncertainty. The ΔQ formula has three subtle failure modes (self-loop double-counting, factor-of-2 in `2m`, γ applied to the wrong term), and Louvain has a confirmed infinite-loop bug in Gonum when ΔQ ≈ float64 noise. Every correctness risk has a specific unit test that catches it. The architecture is straightforward: a single `package graph`, stateless detector structs, per-call `louvainState` via `sync.Pool`, and flat-slice accumulation replacing per-node map allocations.

---

## Key Findings

### Recommended Stack

Stay zero-dependency. All algorithm code imports only `community-detection/graph` and stdlib. Dev-only tooling (`benchstat`, `pprof`) is installed via `go install` and never becomes a module dependency. Go 1.26.1 provides generics, range-over-func, `slices`/`maps` stdlib packages, PGO, and the Go 1.24 `b.Loop()` benchmark API — no external packages are needed for any of these.

**Core technologies:**
- **Go 1.26.1 stdlib only** — zero transitive deps; a library feature, not a limitation
- **`sync.Pool` for `louvainState`** — amortises 5× O(N) slice allocations across concurrent calls
- **`benchstat` (dev install)** — statistical A/B benchmark comparison; standard Go perf tooling
- **`testing.B` + `b.Loop()` (Go 1.24)** — canonical benchmark harness; avoids timer-manipulation pitfalls

### Expected Features

**Must have (table stakes):**
- `CommunityDetector` interface with `Detect(g *Graph) (CommunityResult, error)` — swap-in contract
- `LouvainOptions` / `LeidenOptions` structs — resolution γ, seed, max iterations, tolerance
- `CommunityResult` with `Partition map[NodeID]int`, `Modularity float64`, `Passes int`, `Moves int`
- `DefaultLouvain()` / `DefaultLeiden()` convenience constructors
- Edge-case guards: empty graph, single node, zero-edge graph, negative resolution — no panics
- Concurrent safety: stateless detector receivers; all mutable state per-call or pooled

**Should have (differentiators):**
- Leiden connected-community guarantee — the core reason to use Leiden over Louvain for GraphRAG
- `UseLCC bool` option — Microsoft GraphRAG's own recommendation for sparse KG subgraphs
- Iterative Leiden convergence to γ-dense, subset-optimal partition
- Football (115 nodes) and Polbooks (105 nodes) benchmark fixtures for NMI accuracy validation

**Defer (v2+):**
- Full dendrogram / `DetectHierarchy` — no GraphRAG use case identified
- LFR synthetic benchmark generation — adds code complexity; static fixtures suffice
- Overlapping community detection — incompatible with <100ms target at this scale
- Auto-tuning of resolution γ — callers in deterministic pipelines need explicit control

### Architecture Approach

Stay in a single `package graph`. The existing code, the new `CommunityDetector` interface, both algorithm structs, and all internal state types live together. Splitting into `graph/louvain` and `graph/leiden` sub-packages would require exporting internal state types and add import ceremony with no benefit at this scale. File layout: `detector.go` (interface + result types), `louvain.go` + `louvain_state.go`, `leiden.go` + `leiden_state.go`. All state (`louvainState`, `leidenState`) is unexported.

**Major components:**
1. **`detector.go`** — `CommunityDetector` interface, `CommunityResult`, `LouvainOptions`, `LeidenOptions`, `Partition` type alias
2. **`louvain.go` + `louvain_state.go`** — `LouvainDetector`; stateless receiver; `louvainState` holds flat-slice assignment, totalDegree, internalWeight, neighborWeightBuf (pre-sized, dirty-list reset), nodeDegree cache; pooled via `sync.Pool`
3. **`leiden.go` + `leiden_state.go`** — `LeidenDetector`; embeds `louvainState`; adds `refinedAssignment`, `dirtyQueue`/`dirtySet` for queue-based visiting after first pass
4. **`testdata/fixtures.go`** — Football, Polbooks benchmark graphs; NMI accuracy assertions

### Critical Pitfalls

1. **Self-loop double-counting in ΔQ inner loop (CRIT-01)** — When iterating `g.Neighbors(u)` to accumulate neighbor-community weights, skip edges where `e.To == u`. Failing to do so inflates Q and causes over-merging. Confirmed real bug in Sotera spark-louvain and Gonum.

2. **Factor-of-2 error in `totalWeight` / `2m` (CRIT-02)** — For undirected graphs where `AddEdge(u,v,w)` stores both `adj[u]` and `adj[v]`, iterating `g.Neighbors` sums each edge twice. Compute `totalWeight` from `g.Edges()` (unique edges only). Detection test: single edge `u—v`, two-community partition must give Q = -0.5 exactly.

3. **Tiny ΔQ infinite loop (CRIT-05)** — Using `deltaQ > 0.0` (strict) accepts float64 noise (~2.7e-21) as a valid improvement, causing infinite phase-1 loops. Confirmed bug in gonum/gonum #1488. Fix: always use `deltaQ > opts.Tolerance` (default 1e-7) as the move threshold.

4. **Phase-2 missing self-loop preservation (CRIT-06)** — When building the supergraph, intra-community edge weights must be stored as a self-loop on the supernode, not discarded. Discarding them initialises `Σ_in = 0` on every supernode, causing over-aggressive merging in subsequent passes. Invariant: `ComputeModularity(supergraph, identity) == ComputeModularity(original, phase1Partition)`.

5. **Leiden selective refinement produces disconnected communities (CRIT-08)** — Refining only dirty communities after local-moving can disconnect source communities. Always refine ALL communities after each local-moving phase. Assert connectivity after every Leiden iteration in tests.

---

## Implications for Roadmap

### Phase 1: Interface + Louvain Core
**Rationale:** `CommunityDetector` interface unblocks everything; Louvain is simpler to implement (2-phase vs 3-phase) and validates the data structures before Leiden is built on top.
**Delivers:** Working `LouvainDetector` passing Karate Club accuracy test; `CommunityResult`; all edge-case guards.
**Addresses:** `CommunityDetector` interface, `LouvainOptions`, partition output, empty/single-node/disconnected graph handling, error on invalid resolution.
**Avoids:** CRIT-01 (self-loop skip), CRIT-02 (2m factor), CRIT-03 (γ placement), CRIT-05 (tolerance guard), CRIT-06 (supergraph self-loop).

### Phase 2: Leiden Implementation
**Rationale:** Leiden is the recommended default; it builds on the Louvain phase-1 state but adds refinement. Implementing second means the `louvainState` embedding and `sync.Pool` patterns are already proven.
**Delivers:** Working `LeidenDetector` with connected-community guarantee; `dirtyQueue`-based optimisation; NMI ≥ 0.90 on Football graph.
**Uses:** `leidenState` embedding `louvainState`; same `sync.Pool` pattern; `math/rand/v2` per-call RNG (CONC-04).
**Avoids:** CRIT-08 (refine all communities); assert connectivity after each iteration.

### Phase 3: Performance Hardening + Benchmark Fixtures
**Rationale:** The <100ms / 10K-node target needs measurement before it can be declared met. Performance optimisations (neighborWeightBuf dirty-list, node-degree cache, `sync.Pool` zero-fill) should be validated with benchstat A/B comparisons, not assumed.
**Delivers:** Football and Polbooks fixtures; `BenchmarkLouvain10K` and `BenchmarkLeiden10K` passing 100ms; confirmed `0 allocs/op` after pool warmup; `go test -race` clean.
**Avoids:** PERF-01 (setup outside loop), PERF-02 (sink return values), PERF-03 (full zero-fill in `st.reset`), PERF-04 (-benchmem assertion), CONC-01 (no state on receiver), CONC-03 (clear() full buffer).

### Phase 4: Production-Grade Options + UseLCC
**Rationale:** `UseLCC` is important for production GraphRAG but requires connected-component extraction (a new utility). Deferring it avoids blocking Phases 1–3 on an optional feature.
**Delivers:** `UseLCC bool` option with LCC extraction; isolated nodes absent from `Partition`; documentation of sparse-graph non-reproducibility warning.
**Addresses:** EG-04 (giant component + singletons), sparse KG GraphRAG use case.

### Phase Ordering Rationale

- Louvain before Leiden: same data structures, simpler to validate; Leiden embeds Louvain state
- Interface defined in Phase 1: both detectors are swap-in implementations from the start
- Performance hardening after both algorithms: no point optimising before correctness is confirmed
- `UseLCC` last: depends on a new BFS/DFS component utility; all other features independent

### Research Flags

Phases with well-documented patterns (can skip `/gsd:research-phase`):
- **Phase 1 (Louvain Core):** ΔQ formula is well-specified; PITFALLS.md documents every trap with detection tests
- **Phase 3 (Benchmarking):** Standard Go patterns; STACK.md and PITFALLS.md cover all tooling decisions

Phases that may benefit from targeted research during planning:
- **Phase 2 (Leiden Refinement):** The Traag 2019 paper is clear on algorithm structure, but the arxiv 2402.11454 paper (selective refinement bug) is a 2024 finding — worth reviewing the implementation details carefully before writing code
- **Phase 4 (UseLCC):** Connected-component extraction API design (return type, edge-node mapping back to original graph IDs) has not been researched in detail

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero-dep decision is unambiguous; benchmarking tooling is stdlib + one dev install |
| Features | HIGH (algorithm) / MEDIUM (Go interface) | Algorithm parameters sourced from Traag 2019 and production implementations (Neo4j GDS, NetworkX); Go interface pattern is inference from idiomatic Go |
| Architecture | HIGH (package layout, concurrency) / MEDIUM (Leiden dirty-queue) | sync.Pool and flat-slice patterns are well-established; dirty-queue efficiency claim sourced from GVE-Louvain abstract only |
| Pitfalls | HIGH | CRIT-05 and CRIT-08 are confirmed real bugs with issue tracker citations; CRIT-01/02/06/07 are standard algorithm correctness issues documented in multiple sources |

**Overall confidence:** HIGH

### Gaps to Address

- **NMI accuracy thresholds:** The FEATURES.md recommends NMI ≥ 0.90 on Football with default parameters, but notes MEDIUM confidence because this varies by seed. The exact threshold to assert in CI needs a calibration run before being hardcoded.
- **Directed graph ΔQ (CRIT-07):** PROJECT.md confirms directed graphs are in scope, but the directed modularity formula requires separate out/in-degree tracking. Research covers the formula but implementation complexity is not yet estimated — flag for Phase 1 planning.
- **`g.Edges()` API existence:** The `totalWeight` fix (CRIT-02) assumes `g.Edges()` returns unique edges. Verify that the existing `graph.go` exposes this or plan to add it in Phase 1.

---

## Sources

### Primary (HIGH confidence)
- Traag et al., *Scientific Reports* 2019 — Leiden algorithm, refinement phase, connectivity guarantee
- gonum/gonum issue #1488 — confirmed infinite-loop bug from tiny ΔQ
- Sotera spark-distributed-louvain issue #1 — k_i_in self-loop cross-implementation confirmation
- go.dev/blog/testing-b-loop — Go 1.24 `b.Loop()` guidance
- pkg.go.dev/gonum.org/v1/gonum/graph/community — confirmed Louvain only, no Leiden (2025-12-29)
- Fortunato & Barthélemy, *PNAS* 2007 — resolution limit theorem

### Secondary (MEDIUM confidence)
- arxiv 2402.11454 (2024) — selective Leiden refinement disconnection bug
- arxiv 2312.04876 — GVE-Louvain neighbor accumulator structure (abstract only)
- Neo4j GDS, NetworkX, Graphology docs — interface pattern evidence
- Microsoft GraphRAG discussions — sparse graph non-reproducibility observation

### Tertiary (LOW confidence)
- louvain-igraph docs — default parameter values (MaxMoves=10, Tolerance=1e-7); cross-check with own calibration runs

---

*Research completed: 2026-03-29*
*Ready for roadmap: yes*
