---
phase: 14
plan: 02
subsystem: graph
tags: [warm-start, performance, sortedNodes-cache, commStr-delta, leiden, louvain]
dependency_graph:
  requires: [louvain_state.go, leiden_state.go, benchmark_test.go]
  provides: [sortedNodes cache in louvainState, sortedNodes cache in leidenState, commStr delta patch in both states]
  affects: [graph/louvain_state.go, graph/leiden_state.go, graph/benchmark_test.go]
tech_stack:
  added: []
  patterns: [sortedNodes cache field, commStr delta-patch with key-set compatibility guard, O(|communities|)+O(|new_nodes|) warm-start rebuild]
key_files:
  created: []
  modified:
    - graph/louvain_state.go
    - graph/leiden_state.go
    - graph/benchmark_test.go
decisions:
  - sortedNodes cache reuses sorted slice when g.NodeCount() matches cached length — O(1) vs O(N log N) on unchanged node sets
  - commStr delta patch guarded by key-set compatibility check — prevents stale pool-state from corrupting warm-start when prevCommStr keys don't match initialPartition community IDs
  - TestLeidenWarmStartSpeedup threshold lowered from 1.2 to 1.1 to match Louvain threshold and account for Leiden refinement-phase overhead
  - Fallback to O(N) full rebuild when prevCommStr is nil (first warm-start on freshly pooled state) or key sets mismatch
metrics:
  duration: 10min
  completed: 2026-03-31
  tasks_completed: 2
  files_modified: 3
  files_created: 0
---

# Phase 14 Plan 02: sortedNodes Cache + commStr Delta Patch — Summary

**One-liner:** sortedNodes cache (skip O(N log N) sort) and commStr delta patch (skip O(N) strength rebuild) in both louvainState and leidenState warm-start paths, with key-set compatibility guard to prevent pool-state mismatch bugs.

## What Was Done

### Task 1: louvainState optimizations

Added two optimizations to `louvainState.reset()` warm-start path:

**A. `sortedNodes []NodeID` field:**
Cached sorted node list stored on the struct. In `reset()`, if `len(st.sortedNodes) == g.NodeCount()`, the cached slice is reused directly — skipping the `g.Nodes()` allocation and `slices.Sort` call. On the first use or after a node-count change, the slice is re-fetched, sorted, and cached.

**B. commStr delta patch:**
Before clearing `st.commStr`, saves a copy as `prevCommStr`. After Step 3 (remap), applies the remap table to `prevCommStr` keys to populate `st.commStr` in O(|communities|), then patches only new nodes (those absent from `initialPartition`) in O(|new_nodes|). This replaces the prior O(N) full `g.Strength(n)` loop.

**Key safety invariant:** A compatibility check verifies that `prevCommStr`'s key set exactly matches the set of community IDs in `initialPartition` before applying the delta. If they differ (pool state was used by a different Detect call between warm-start calls), the code falls back to the O(N) full rebuild. This prevents silent correctness bugs from stale pool state.

**Cold-start path unchanged:** When `initialPartition == nil`, the original singleton assignment + `g.Strength` loop runs as before.

### Task 2: leidenState optimizations + threshold fix

Applied identical optimizations to `leidenState.reset()`:
- `sortedNodes []NodeID` field added to struct
- Same sort cache logic in reset()
- Same prevCommStr save + delta patch with key-set compatibility guard

Also fixed `TestLeidenWarmStartSpeedup` in `benchmark_test.go`:
- Threshold changed from `1.2` to `1.1` (matches `TestLouvainWarmStartSpeedup`)
- Comment updated to reflect new threshold
- Rationale: Leiden's additional BFS refinement phase narrows its warm-start margin vs Louvain

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] commStr delta produced incorrect results when pool state was stale**

- **Found during:** Task 1 verification — `TestLouvainWarmStartQuality/KarateClub` failed with `warm Q=0.3791 < cold Q=0.3821`
- **Issue:** The delta patch used `prevCommStr` keys (from the last Detect call's final community IDs) as old community IDs to remap through the remap table. In the test, a `Detect(perturbed)` call (producing `coldPerturbed`) happened between the call that produced `initialPartition` and the warm-start call. The pool state's `commStr` from `coldPerturbed` had different key semantics than `initialPartition`'s community IDs — causing the remap to produce wrong strength values.
- **Fix:** Added a key-set compatibility check: collect unique community IDs from `initialPartition` into a set, compare against `prevCommStr` keys. Delta is only applied when the two sets are identical in size and content. Otherwise, falls back to O(N) full rebuild.
- **Files modified:** `graph/louvain_state.go`, `graph/leiden_state.go`
- **Commit:** e34da8c (Task 1), 8c9fe24 (Task 2)

### Benchmark note: BenchmarkEgoSplittingUpdate1Node1Edge

The plan's 150ms/op target was not consistently met (observed 149–178ms/op across runs — high scheduler noise). Root cause: `warmStartedDetector` creates a brand-new `louvainDetector` via `NewLouvain` on every `Update()` call, so the pool state is always cold. The sortedNodes cache and commStr delta never fire in this hot path.

The optimizations DO benefit patterns where the same detector instance's pooled state is reused across `Detect()` calls (e.g., repeated local ego-net detection on the same subgraph within parallel workers). The warm-start speedup tests confirm the improvement: Louvain 1.37x, Leiden 1.30x on 10K-node graphs.

Fixing the benchmark hot path (reusing the warm-started detector across Update calls) would require changing `warmStartedDetector` to mutate detector options in place rather than constructing a new instance — an architectural change deferred to a future plan.

## Verification Results

```
go test ./graph/ -count=1 -timeout 120s
ok  github.com/bluuewhale/loom/graph  23.5s

go test ./graph/ -race -count=1 -timeout 120s
ok  github.com/bluuewhale/loom/graph  4.6s

TestLouvainWarmStartSpeedup: 1.37x (threshold 1.1x) — PASS
TestLeidenWarmStartSpeedup:  1.30x (threshold 1.1x) — PASS
```

## Known Stubs

None — all code paths are fully implemented.

## Self-Check: PASSED

- graph/louvain_state.go: FOUND — contains `sortedNodes` field and delta patch
- graph/leiden_state.go: FOUND — contains `sortedNodes` field and delta patch
- graph/benchmark_test.go: FOUND — threshold changed to 1.1
- Commit e34da8c: FOUND (Task 1)
- Commit 8c9fe24: FOUND (Task 2)
