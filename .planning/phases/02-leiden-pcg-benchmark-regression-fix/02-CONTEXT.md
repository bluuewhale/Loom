# Phase 2: Leiden PCG benchmark regression fix - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — discuss skipped)

<domain>
## Phase Boundary

Eliminate per-community map allocations in refinePartition — the dominant Leiden-specific allocation source — to bring Leiden 10K allocs/op to Louvain parity.

Success criteria:
- Leiden 10K allocs/op drops from 58,220 to ≤ 46,500 (Louvain parity + 10% margin; seed 110 PCG 4-pass)
- All existing tests pass
- No public API signature changes

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase (performance optimization, technical success criteria only, no user-facing behavior).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- CSR adjacency view from Phase 1 (csr.adjByIdx[])
- sync.Pool for state reuse (leidenState)
- refinePartition existing implementation as reference

### Established Patterns
- Pool scratch slices grown lazily (first use per pool lifetime)
- PCG rand/v2 zero-alloc reseed pattern from Phase 1
- BFS cursor pattern (head int) instead of queue[1:] slicing

### Integration Points
- refinePartition called from leidenDetector.Detect()
- leidenState pool manages scratch memory lifetime

</code_context>

<specifics>
## Specific Ideas

Replace per-community `map[NodeID]bool` allocations in refinePartition with CSR-indexed `[]bool` scratch slices + sorted `[]commNodePair` pairs.

</specifics>

<deferred>
## Deferred Ideas

None — infrastructure phase.

</deferred>
