# Phase 3: Leiden BFS Refinement Speed - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — implementation already committed)

<domain>
## Phase Boundary

Reduce ns/op overhead in `refinePartitionInPlace` by replacing O(N log N) comparison sort with O(N) counting sort, and replacing `g.Neighbors()` adjacency map lookup with `csr.adjByIdx[]` direct slice access. Pure performance optimization — no user-facing behavior changes.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure/performance optimization phase. Implementation already committed in feat/graph-core-optimization branch (commit f476276).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `LeidenState` scratch buffers: `commCountScratch []int`, `commSeenComms []int`, `commSortedPairs []commNodePair`, `bfsQueue []int32` added in leiden_state.go
- CSR adjacency: `csr.adjByIdx[]` direct slice access available from Phase 1 CSR work

### Established Patterns
- Zero-alloc refinePartitionInPlace established in Phase 2
- Counting sort with sparse reset pattern matches existing scratch buffer conventions
- int32 CSR indices match existing CSR infrastructure

### Integration Points
- `graph/leiden.go`: `refinePartitionInPlace` function
- `graph/leiden_state.go`: `LeidenState` struct scratch buffers

</code_context>

<specifics>
## Specific Ideas

Implementation completed in commit f476276:
- Counting sort replaces `slices.SortFunc` — eliminates O(N log N) sort comparator overhead
- BFS queue stores int32 CSR indices — eliminates `mapaccess2_fast64` per BFS dequeue
- Hardening: bounds assertion added, amortized growth for commSortedPairs

</specifics>

<deferred>
## Deferred Ideas

None — phase focused solely on counting sort + CSR adjacency optimizations.

</deferred>
