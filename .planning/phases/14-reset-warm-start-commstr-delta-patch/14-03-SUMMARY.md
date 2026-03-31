---
phase: 14
plan: 03
subsystem: graph
tags: [warm-start, performance, ego-splitting, detector-reuse]
dependency_graph:
  requires: [ego_splitting.go, ego_splitting_test.go, louvain_state.go, leiden_state.go]
  provides: [warmStartedDetector in-place mutation semantics]
  affects: [graph/ego_splitting.go, graph/ego_splitting_test.go]
tech_stack:
  added: []
  patterns: [in-place mutation of detector opts, pool-state reuse across Update() calls]
key_files:
  created: []
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go
decisions:
  - warmStartedDetector mutates detector.opts.InitialPartition in place and returns same instance — enables pool-warm state accumulation across consecutive Update() calls
  - TestWarmStartedDetector_DoesNotMutateOriginal replaced with TestWarmStartedDetector_MutatesInPlace — old test verified the opposite of the new intended behavior
  - 150ms/op benchmark target not met by this change alone — root cause is the double-reset in acquireLouvainState (pre-reset with nil initialPartition) and Louvain phase1 on 10K persona graph; documented as deferred
metrics:
  duration: 34min
  completed: 2026-03-31
  tasks_completed: 1
  files_modified: 2
  files_created: 0
---

# Phase 14 Plan 03: Cache Warm-Start Detectors — Summary

**One-liner:** warmStartedDetector now mutates InitialPartition in place on the existing detector instance, enabling pool-warm state (sortedNodes cache, commStr) to persist across consecutive Update() calls.

## What Was Done

### Task 1: Cache warm-start detectors via in-place InitialPartition mutation

**ego_splitting.go — warmStartedDetector():**

Changed from constructing a new detector (`NewLouvain(LouvainOptions{...})`) to mutating the existing detector's `opts.InitialPartition` field in place and returning the same pointer:

```go
func warmStartedDetector(d CommunityDetector, partition map[NodeID]int) CommunityDetector {
    switch det := d.(type) {
    case *louvainDetector:
        det.opts.InitialPartition = partition
        return det
    case *leidenDetector:
        det.opts.InitialPartition = partition
        return det
    default:
        return d
    }
}
```

Updated the doc comment to reflect mutation semantics.

**ego_splitting_test.go — warmStartedDetector tests:**

- `TestWarmStartedDetector_Louvain`: added `result == base` same-pointer assertion; kept opts field and InitialPartition content assertions
- `TestWarmStartedDetector_Leiden`: added `result == base` same-pointer assertion; kept opts field assertions
- `TestWarmStartedDetector_NilPartition`: added `result == base` same-pointer assertion
- `TestWarmStartedDetector_DoesNotMutateOriginal` → **replaced** with `TestWarmStartedDetector_MutatesInPlace`: verifies first call sets InitialPartition, second call with nil clears it

## Deviations from Plan

None in terms of implementation scope. The plan's fix was implemented exactly as specified.

### Performance Gap (Not a Deviation — Documented as Known)

The plan's acceptance criterion of `BenchmarkEgoSplittingUpdate1Node1Edge <= 150ms/op` was not met.

**Measured results (Apple M4, -benchtime=3s -count=5):**
- 161ms, 162ms, 167ms, 183ms, 165ms — median ~164ms

**Root cause analysis:**

The plan expected in-place mutation to enable pool-warm state to "accumulate" across `Update()` calls. The mechanism is correct: the same `*louvainDetector` instance is now reused, so `sync.Pool` can return the same pool entry across benchmark iterations. However, `acquireLouvainState` calls `st.reset(g, seed, nil)` as a pre-reset before `Detect` resets with `InitialPartition`. This double-reset means:

1. Pre-reset (nil): O(N log N) sort + O(N) commStr rebuild — fires every call
2. Real reset (warm): O(1) sort (cache hit from pre-reset) + O(|communities|) commStr delta

The delta patch DOES fire (saving the second O(N) commStr rebuild), and the sortedNodes cache DOES save the second sort. But the pre-reset's O(N log N) + O(N) work dominates. The net saving is ~3ms (167ms → 164ms).

**What was attempted:**
1. Tried removing the pre-reset from `acquireLouvainState` entirely — caused `TestLouvainWarmStartQuality/KarateClub` to fail (warm Q < cold Q) because stale pool state from unrelated Detect calls contaminated the commStr delta patch via a false-positive key-set compatibility check.
2. Tried threading `initialPartition` through `acquireLouvainState` to do a single warm reset — same test failure for the same reason.
3. Both attempts reverted to preserve correctness.

**Conclusion:** The 150ms target requires either (a) a correctness-safe mechanism to skip the pre-reset entirely, or (b) a different optimization approach (e.g., persona graph size reduction, Louvain early-exit). Deferred.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: graph/ego_splitting.go (modified — warmStartedDetector in-place mutation)
- FOUND: graph/ego_splitting_test.go (modified — MutatesInPlace test, same-pointer assertions)
- FOUND: commit 583f19a (task commit)
- go test ./graph/ -count=1 -timeout=120s: PASS
- go test ./graph/ -race -count=1 -timeout=120s: PASS
- BenchmarkEgoSplittingUpdate1Node1Edge: 161-167ms/op (target 150ms — NOT MET)
