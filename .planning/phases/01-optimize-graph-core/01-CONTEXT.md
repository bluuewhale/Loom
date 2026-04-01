# Phase 1: optimize graph core - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — optimization/performance, no user-facing behavior)

<domain>
## Phase Boundary

Reduce allocation counts and improve throughput in the core graph hot paths without changing any public API. Target the top-ranked bottlenecks from the codebase optimization audit: Nodes() caching, CSR adjacency view for the phase1 hot path, BFS cursor fix, buildSupergraph canonical dedup, Subgraph seen-map reuse, rand.Rand/rng reuse, and removal of dead code (deltaQ, Tolerance no-op field). All existing tests must continue to pass.

**Out of scope:** OmegaIndex O(n²) fix (test-only, not hot path), EgoSplitting channel batching, louvainState aliasing refactor, warm-start compaction skip (these are follow-on work).

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure/performance phase. Use CONCERNS.md analysis as the authoritative spec. Priority order: high-impact first (Nodes caching, CSR view), then medium (BFS cursor, buildSupergraph, Subgraph), then low-medium (rand reuse), then dead code cleanup.

### Execution approach
- Implement as atomic commits per optimization area
- Run `go test ./graph/...` and `go test -bench=. -benchmem ./graph/...` to validate before/after
- Each optimization must not regress existing tests

### CSR scope
- CSR (Compressed Sparse Row) is an internal view only — built at the start of Detect(), not stored on Graph struct
- Public Graph API (AddNode, AddEdge, Neighbors, Nodes) remains unchanged
- CSR used only in phase1 hot path inside louvain.go/leiden.go

### Nodes() caching
- Cache `sortedNodes []NodeID` on Graph struct, invalidated (nil) on AddNode/AddEdge
- Callers must not modify the returned slice (documented contract)
- Zero-alloc on repeated calls

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `sync.Pool` already used for louvainStatePool and leidenStatePool (louvain_state.go:21, leiden_state.go:23)
- `louvainState` and `leidenState` structs already have `neighborBuf` and `commStr` precomputed caches
- `normalizePartition` already produces 0-indexed contiguous NodeIDs (supergraph base)

### Established Patterns
- State reset pattern: `reset(g *Graph, seed int64, initial Partition)` — modify in place
- Pool pattern: `Put`/`Get` with full reset on Get
- Test helpers in `testhelpers_test.go` for reuse across test files
- Benchmark fixtures: `barabásiAlbert`, `karate`, `football` graphs in `benchmark_test.go`

### Integration Points
- `graph/graph.go` — Graph struct, Nodes(), Neighbors(), Subgraph()
- `graph/louvain.go` — phase1, buildSupergraph, normalizePartition, deltaQ (dead)
- `graph/leiden.go` — refinePartition BFS queue
- `graph/louvain_state.go` — reset(), rand.New(src)
- `graph/leiden_state.go` — reset(), rand.New(src)

### Baseline Metrics (bench-baseline.txt)
- Louvain 1K: ~5.4ms, 5 217 allocs/op
- Leiden 1K: ~5.5ms, 7 248 allocs/op
- Louvain 10K: ~62ms, 48 773 allocs/op
- Leiden 10K: ~67ms, 66 524 allocs/op
- Louvain WarmStart 10K: ~46ms, 25 754 allocs/op

</code_context>

<specifics>
## Specific Ideas

From CONCERNS.md:

1. **Nodes() cache** — `sortedNodes []NodeID` field on Graph, nil on mutation, populated+cached on first call. Zero-alloc contract for callers.
2. **CSR view** — `type csrGraph struct { offsets []int32; edges []Edge }` built once in Detect(). `csrNeighbors(n NodeID)` returns `edges[offsets[n]:offsets[n+1]]`. Replaces map lookup in phase1 inner loop.
3. **BFS cursor** — Replace `queue = queue[1:]` with `head int` cursor in leiden.go:refinePartition. No change to algorithm, eliminates backing-array leak.
4. **buildSupergraph dedup** — Add `lo < hi` guard when iterating neighbors to process each undirected edge once. Eliminates divide-by-2 correction. Pre-size maps.
5. **Subgraph seen-map** — Pool the `seen map[[2]NodeID]struct{}` via sync.Pool in graph.go, or use sorted-slice for small neighborsets.
6. **rand.Rand reuse** — Store `rng *rand.Rand` on state, reseed via `rng.Seed(newSeed)` instead of `rand.New(src)` on every reset.
7. **Dead code** — Delete deltaQ function (louvain.go:261-270). Add comment to CommStrength about O(n) cost.

</specifics>

<deferred>
## Deferred Ideas

- OmegaIndex co-occurrence rewrite — test-only, not hot path
- EgoSplitting channel batching — separate phase
- louvainState aliasing refactor (phaseState interface) — structural refactor, separate phase
- Warm-start compaction skip — requires contiguous-partition contract design
- Tolerance-based early exit — feature work, separate phase
- `SubgraphInto(dst *Graph)` API — API addition, separate phase

</deferred>
