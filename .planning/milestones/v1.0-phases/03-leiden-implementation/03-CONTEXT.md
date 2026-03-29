# Phase 03: Leiden Implementation - Context

**Gathered:** 2026-03-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 03 delivers a complete `LeidenDetector` implementation that produces connected communities (no disconnected nodes in a community). Leiden improves on Louvain by adding a refinement phase that ensures internal connectivity. The implementation reuses Louvain helpers (`phase1`, `deltaQ`, `buildSupergraph`, `normalizePartition`) and provides a drop-in swap via `CommunityDetector`. Accuracy target: NMI ≥ 0.7 vs. Karate Club ground truth, Q > 0.35.

</domain>

<decisions>
## Implementation Decisions

### Refinement Phase Algorithm
- Connectivity detection: BFS within each community — simple, readable, fits small-to-medium community sizes
- Disconnected sub-communities: split each connected component into its own community (components become separate communities after refinement)
- Refinement timing: run every iteration (after every local-move pass) — this is the true Leiden behavior per Traag et al. (2019)
- Aggregation partition: use the refined partition (not local-move partition) for supergraph construction — this is the key correctness requirement that distinguishes Leiden from Louvain

### Code Structure & Reuse
- Reuse Louvain helpers directly: `phase1`, `deltaQ`, `buildSupergraph`, `normalizePartition`, `reconstructPartition` — same package, no visibility barriers, no duplication
- New `leidenState` struct in `leiden_state.go` with its own fields including `refinedPartition map[NodeID]int` alongside the standard partition/commStr/rng fields
- Files: `leiden.go` + `leiden_state.go` (mirrors the Louvain file split)

### NMI & Test Infrastructure
- NMI implementation: unexported test helper `nmi(p1, p2 map[NodeID]int) float64` inside `leiden_test.go` — keeps it test-only, no public API surface added
- Test file: separate `leiden_test.go` (mirrors `louvain_test.go` structure)
- Ground-truth fixture: reuse existing `KarateClub()` and `KarateGroundTruth()` from `graph/testdata/karate.go`

### Claude's Discretion
- Internal `leidenState` field names and BFS helper implementation details
- Whether to use a visited map or slice for BFS queue
- Self-loop and isolated node handling in BFS (skip self-loops; isolated nodes stay in singleton communities)
- How `refinedPartition` is initialized (copy of local-move partition, then split)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `phase1(g *Graph, state *louvainState, resolution, m float64) int` — Phase 1 local move, fully reusable
- `deltaQ(g, n, comm, partition, commStr, resolution, m)` — ΔQ formula helper, fully reusable
- `buildSupergraph(g *Graph, partition map[NodeID]int) (*Graph, map[NodeID]NodeID)` — supergraph compression, reusable with refined partition
- `normalizePartition(partition map[NodeID]int) map[NodeID]int` — final normalization, reusable
- `reconstructPartition(origNodes, nodeMapping, superPartition)` — original → final community mapping, reusable
- `g.Nodes() []NodeID`, `g.Neighbors(id) []Edge`, `g.Strength(n)`, `g.TotalWeight()`, `g.IsDirected()` — graph API
- `KarateClub()` + `KarateGroundTruth()` from `graph/testdata/karate.go` — accuracy verification fixture

### Established Patterns
- Constructor pattern: `NewLeiden(opts LeidenOptions) CommunityDetector` (stub already in `detector.go`)
- Zero-value defaults: Resolution 0.0 → 1.0, Seed 0 → random, MaxIterations 0 → unlimited, Tolerance 0.0 → 1e-7
- Same guard clauses as Louvain: empty graph, single node, TotalWeight=0, directed graph
- Insertion sort (no `sort` package import) for deterministic ordering
- `louvainState` as model for `leidenState` structure in `leiden_state.go`

### Integration Points
- `detector.go`: `leidenDetector.Detect()` stub to be replaced with real implementation
- `leiden.go`: main `Detect` method on `*leidenDetector`
- `leiden_state.go`: `leidenState` struct + `newLeidenState(g, seed)` constructor
- `leiden_test.go`: new test file with NMI helper + Karate Club accuracy test

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond REQUIREMENTS.md — open to standard Leiden implementation approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
