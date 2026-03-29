# Phase 02: Interface + Louvain Core - Context

**Gathered:** 2026-03-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 02 delivers the `CommunityDetector` interface, `CommunityResult`, option structs, and a complete Louvain algorithm implementation. Callers can run community detection on any undirected graph via a swappable interface. Louvain handles all edge cases (empty, single-node, disconnected) without panic. Directed graphs return `ErrDirectedNotSupported` — directed ΔQ deferred to v2.

</domain>

<decisions>
## Implementation Decisions

### Options Zero-Value Semantics
- `Resolution == 0.0` → auto-default to 1.0 (callers cannot meaningfully pass literal zero resolution)
- `MaxPasses == 0` → unlimited passes; algorithm terminates via tolerance convergence
- `Tolerance == 0.0` → auto-default to 1e-7 (literal zero tolerance would prevent early termination)
- `Seed == 0` → use random seed (non-deterministic by default; callers who want repeatability set explicit seed)

### Detect() Error Contract
- Empty graph (0 nodes) → `(CommunityResult{}, nil)` — empty partition is valid; matches LOUV-05 "no error" spec
- Single-node graph → valid result with `Partition: {id: 0}`, `Modularity: 0.0`, `nil` error
- Directed graph → return `ErrDirectedNotSupported` sentinel error (directed support deferred to v2 per REQUIREMENTS)
- `CommunityResult.Passes` → always count executed passes; `1` even for trivial single-pass convergence

### Louvain Algorithm Behavior
- Node visit order each pass: random shuffle driven by `Seed` — avoids map-iteration bias; Seed=0 produces random order
- Supergraph node IDs: contiguous new IDs after compression — internal only, no original→super mapping needed
- ΔQ computation: extracted as private `deltaQ(...)` helper function — separates formula from iteration logic
- Final partition normalization: normalize to 0-indexed contiguous integers before returning to caller

### Claude's Discretion
- Internal `louvainState` struct layout and field names
- Self-loop weight accumulation in supergraph construction
- How community strength sums are cached during phase 1 (map vs recompute)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `g.Nodes() []NodeID` — iterate all nodes in graph (used for phase 1 loop)
- `g.Neighbors(id) []Edge` — adjacency list per node (used for ΔQ neighbor scan)
- `g.Strength(n NodeID) float64` — sum of incident edge weights (k_i in modularity formula)
- `g.TotalWeight() float64` — total edge weight m (2m denominator in modularity)
- `g.IsDirected() bool` — guard for `ErrDirectedNotSupported` check
- `g.WeightToComm(n, comm, partition)` — already implemented; reuse for k_i_in calculation in ΔQ
- `g.CommStrength(comm, partition)` — already implemented; reuse for Σ_tot in ΔQ formula
- `NewGraph(directed bool) *Graph` — used to construct supergraph in phase 2

### Established Patterns
- Constructor pattern: `NewLouvain(opts LouvainOptions) CommunityDetector` matches `NewGraph`, `NewRegistry`
- Zero-error sentinel returns for degenerate inputs (e.g., `ComputeModularity` returns 0.0 on empty graph)
- No external dependencies — keep Louvain pure stdlib; `math/rand` for shuffle
- Doc comments on every exported type/function (godoc convention)
- Unexported struct fields, exported types only

### Integration Points
- New files: `graph/detector.go` (interface + types), `graph/louvain.go` + `graph/louvain_state.go`
- New test file: `graph/louvain_test.go`
- Existing `graph/testdata/karate.go` — primary accuracy fixture for Louvain correctness test
- `ComputeModularity` / `ComputeModularityWeighted` — used in tests to validate returned `Modularity` field

### Blocker Resolution
- STATE.md flagged: "Verify `g.Edges()` API exists" — **resolved**: no `Edges()` method; iterate via `g.Nodes()` + `g.Neighbors(id)` instead
- Directed graph ΔQ formula: **resolved** — return `ErrDirectedNotSupported` for Phase 02; defer to v2

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond what is in REQUIREMENTS.md — open to standard Louvain implementation approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
