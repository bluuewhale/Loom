---
phase: 03-leiden-bfs-refinement-speed-linear-grouping-csr-adjacency
plan: "01"
subsystem: algorithm
tags: [leiden, csr, counting-sort, bfs, performance]

# Dependency graph
requires:
  - phase: 02-leiden-pcg
    provides: zero-alloc refinePartitionInPlace with CSR bool scratch + sorted commNodePairs
  - phase: 01-optimize-graph-core
    provides: CSR adjacency structure (csr.adjByIdx[]) and leidenState scratch buffer pattern
provides:
  - O(N) counting sort replacing O(N log N) slices.SortFunc in refinePartitionInPlace
  - int32 CSR BFS queue eliminating mapaccess2_fast64 per BFS dequeue
  - commCountScratch/commSeenComms/commSortedPairs/bfsQueue fields on leidenState
  - bounds assertion on commCountScratch index (future regression guard)
affects: [leiden-benchmarks, refinePartitionInPlace callers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Counting sort with sparse reset: commSeenComms tracks dirty entries for O(touched) reset"
    - "int32 CSR index queue: store dense index instead of NodeID to enable direct csr.adjByIdx[] access"
    - "Amortized growth (+25% headroom) on commSortedPairs realloc"

key-files:
  created: []
  modified:
    - graph/leiden.go
    - graph/leiden_state.go

key-decisions:
  - "Counting sort uses commSeenComms dirty list for sparse reset — avoids O(N) full zeroing each call"
  - "BFS queue stores int32 CSR dense indices: csr.adjByIdx[curIdx] is a direct slice access vs g.Neighbors() map lookup"
  - "bounds panic on commCountScratch: if comm < 0 || comm >= n — catches future phase1 regressions early"
  - "commSortedPairs grows with +25% headroom on realloc to amortize future reallocations"

patterns-established:
  - "Sparse reset pattern: maintain dirty list (commSeenComms), zero only touched entries at end of call"
  - "CSR index queue: prefer int32 CSR dense indices over NodeID for BFS inner loops to eliminate map lookup"

requirements-completed: []

# Metrics
duration: ~15min
completed: 2026-04-01
---

# Phase 3 Plan 01: counting sort + CSR adjacency in refinePartitionInPlace Summary

**O(N) counting sort with sparse reset replaces slices.SortFunc, and int32 CSR BFS queue eliminates mapaccess2_fast64 per dequeue — Leiden 10K 60.4ms → 59.1ms (−2.2%)**

## Performance

- **Duration:** ~15 min
- **Started:** 2026-04-01T18:00:00Z
- **Completed:** 2026-04-01T18:22:00Z
- **Tasks:** 1 (implementation + hardening combined)
- **Files modified:** 2

## Accomplishments

- Replaced `slices.SortFunc` (O(N log N)) with counting sort (O(N)) using sparse-reset `commCountScratch` — eliminates sort comparator overhead (`slices.partitionCmpFunc` was 3.4% flat in CPU profile)
- Replaced `[]NodeID` BFS queue with `[]int32` CSR dense indices — `csr.adjByIdx[curIdx]` direct slice access replaces `g.Neighbors()` map lookup, eliminating one `mapaccess2_fast64` per BFS dequeue
- Added hardening: bounds panic on `commCountScratch` index, amortized growth (+25% headroom) on `commSortedPairs`, explicit ordering invariant comment coupling `commSeenComms` sort order to `commSortedPairs` layout
- Leiden 10K gap vs Louvain narrowed from 7.5% to 5.2% (−2.3 percentage points)

## Task Commits

Implementation committed atomically:

1. **counting sort + CSR BFS adjacency** - `f476276` (perf)

## Files Created/Modified

- `graph/leiden.go` — rewrote `refinePartitionInPlace`: counting sort pass 1/2, prefix-sum scatter, sparse reset, int32 CSR BFS queue; added `"fmt"` import for bounds panic
- `graph/leiden_state.go` — added `commCountScratch []int`, `commSeenComms []int`, `commSortedPairs []commNodePair`, `bfsQueue []int32` fields to `leidenState`; initialized in `newLeidenState()`

## Decisions Made

- **Counting sort with sparse reset:** `commSeenComms` dirty list tracks touched community IDs so only those entries are zeroed after each call — avoids full O(N) zero pass per invocation
- **int32 CSR index queue:** storing CSR dense indices instead of NodeIDs enables `csr.adjByIdx[curIdx]` direct slice access, eliminating one map lookup per BFS step
- **Bounds assertion:** `if comm < 0 || comm >= n { panic(...) }` guards `commCountScratch` — catches future regressions where phase1 produces partition IDs outside [0, N) before they corrupt the counting sort
- **Amortized growth:** `commSortedPairs` grows at +25% headroom on realloc — matches Go append semantics for amortized O(1) growth

## Deviations from Plan

None — plan executed exactly as written. Implementation was pre-committed in `f476276` before plan execution; this plan documents the work post-hoc.

## Issues Encountered

None — all tests pass, benchmark improvement confirmed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `refinePartitionInPlace` is now at Louvain performance parity on allocs; ns/op gap narrowed to ~5%
- Future optimization opportunities: profile remaining hot paths in Leiden's phase1 (move/swap loop), or additional BFS inner loop micro-optimizations
- No blockers for downstream work

---
*Phase: 03-leiden-bfs-refinement-speed-linear-grouping-csr-adjacency*
*Completed: 2026-04-01*
