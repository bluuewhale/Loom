# Phase 08: Full Detect Pipeline + Accuracy + Performance - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Wire the Phase 07 helpers (`buildPersonaGraph`, `mapPersonasToOriginal`) into `EgoSplittingDetector.Detect()`, implement `OmegaIndex` as a new exported function in `graph/omega.go`, validate accuracy on three fixture graphs (Karate Club, Football, Polbooks), assert race-safety with `go test -race`, and add a 10K-node benchmark.

Phase 07's `Detect()` stub returning `ErrNotImplemented` is replaced. No other existing files touched.

</domain>

<decisions>
## Implementation Decisions

### Detect() Pipeline Wiring
- Sequence: `buildPersonaGraph(g, opts.LocalDetector)` → `opts.GlobalDetector.Detect(personaGraph)` → `mapPersonasToOriginal(globalResult.Partition, inverseMap)` → build `OverlappingCommunityResult`
- Error handling: propagate errors from `buildPersonaGraph` and `GlobalDetector.Detect` — return `(OverlappingCommunityResult{}, err)` on first error
- Directed graph guard: `if g.IsDirected() { return OverlappingCommunityResult{}, ErrDirectedNotSupported }` — mirrors CommunityDetector convention
- Building Communities from NodeCommunities: invert `map[NodeID][]int` → `[][]NodeID` using a size-tracked make pass (count communities first, then fill)

### OmegaIndex Implementation
- File: new `graph/omega.go` — keeps accuracy metric separate from algorithm file
- Signature: `OmegaIndex(result OverlappingCommunityResult, groundTruth [][]NodeID) float64` — matches EGO-08 exactly
- Formula: Collins & Dent (1988) pair-counting Omega index — for each node pair (u,v), count t_k = number of communities they co-appear in for result and for groundTruth; Omega = (Observed − Expected) / (1 − Expected) where Observed = fraction of pairs with same t_k in both, Expected = sum_k(p_k^2) where p_k = fraction of pairs with t_k co-memberships by chance
- Ground truth conversion: caller wraps fixture `Partition map[int]int` into `[][]NodeID` (one slice per community); OmegaIndex accepts `[][]NodeID` directly and converts internally to `map[NodeID][]int` membership map

### Accuracy Threshold Strategy
- Initial test: assert `OmegaIndex >= 0.0` and log actual score with `t.Logf("Omega index: %f", score)`
- After first run: update threshold to `>= 0.5` once actual scores are confirmed achievable (per STATE.md note: calibrate empirically)
- If 0.5 is not achievable on first run, log the gap and adjust — do not block the phase on an uncalibrated threshold

### Benchmark
- Function: `BenchmarkEgoSplitting10K` in `graph/ego_splitting_test.go`
- Graph: deterministic Erdős–Rényi with fixed seed, 10K nodes, ~5 edges per node (50K total edges) — matches prior benchmark pattern in `benchmark_test.go`
- Assert: benchmark completes ≤ 300ms/op

### Race Safety
- Design: each `Detect` call allocates fresh maps — no shared mutable state → inherently race-safe
- Validation: `go test -race ./...` as the race check

### Claude's Discretion
- Exact Omega formula implementation details (indexing, floating-point edge cases)
- Benchmark graph generation helper (can reuse from existing benchmark_test.go patterns)
- Whether to add a `go:generate` comment for benchmark graph if it takes long to build

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `graph/ego_splitting.go:Detect()` — stub to replace (keep same signature)
- `graph/ego_splitting.go:buildPersonaGraph()` and `mapPersonasToOriginal()` — Phase 07 helpers
- `graph/detector.go:ErrDirectedNotSupported` — reuse for directed graph guard
- `graph/testdata/karate.go:KarateClubEdges`, `KarateClubPartition` — 34 nodes
- `graph/testdata/football.go:FootballEdges`, `FootballPartition` — 115 nodes, 12 communities
- `graph/testdata/polbooks.go:PolbooksEdges`, `PolbooksPartition` — 105 nodes, 3 communities
- `graph/modularity.go:ComputeModularity` — reference for how accuracy metrics are structured
- `benchmark_test.go` — existing benchmark patterns (random graph generation with seed, b.ResetTimer())

### Established Patterns
- New concept → new file (`omega.go` for `OmegaIndex`, mirrors `modularity.go` for `ComputeModularity`)
- Exported accuracy functions in `package graph`, doc comment starting with identifier name
- Benchmark: `func BenchmarkXxx(b *testing.B)`, `b.ResetTimer()` before timed section, `b.N` loop
- Test fixtures: load via `testdata.KarateClubEdges` etc., add nodes via `g.AddNode`, add edges via `g.AddEdge`

### Integration Points
- `graph/ego_splitting.go` — Detect() replaces stub
- `graph/omega.go` — new file for OmegaIndex
- `graph/ego_splitting_test.go` — accuracy tests + benchmark added here
- No changes to `graph/detector.go`, `graph/graph.go`, or any testdata fixture

</code_context>

<specifics>
## Specific Ideas

- The `mapPersonasToOriginal` helper from Phase 07 returns `map[NodeID][]int` (original NodeID → list of community indices from global partition). Building `OverlappingCommunityResult.Communities` requires inverting this: create `[][]NodeID` where `Communities[i]` contains all NodeIDs whose `NodeCommunities` includes index `i`.
- Fixture ground truth conversion pattern: `partition[Karate/Football/Polbooks]` is `map[int]int` (nodeID → communityID). Convert to `[][]NodeID` by iterating and grouping by communityID, then pass to `OmegaIndex`.
- The STATE.md blocker about Omega threshold is addressed by the log-first strategy — run, observe, then set threshold.

</specifics>

<deferred>
## Deferred Ideas

- Parallel ego-net construction (goroutine pool) — explicitly deferred per REQUIREMENTS.md
- Overlapping NMI (McDaid 2011) — deferred to v1.3 per REQUIREMENTS.md
- Best-match F1 metric — deferred to v1.3
- LFR synthetic benchmark fixtures with known overlapping ground truth — deferred to v1.3

</deferred>
