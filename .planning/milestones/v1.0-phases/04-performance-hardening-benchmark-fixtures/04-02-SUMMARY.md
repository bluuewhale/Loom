---
phase: 04-performance-hardening-benchmark-fixtures
plan: 02
subsystem: graph
tags: [go, community-detection, performance, sync.Pool, benchmark, benchstat, race-detector]

requires:
  - phase: 02-interface-louvain-core
    provides: Louvain algorithm + CommunityDetector interface
  - phase: 03-leiden-implementation
    provides: Leiden algorithm
  - phase: 04-01
    provides: test fixtures, NMI accuracy suite

provides:
  - sync.Pool state reuse for Louvain and Leiden
  - neighborBuf single-pass accumulation in phase1
  - BenchmarkLouvain10K, BenchmarkLeiden10K (< 100ms/op)
  - TestConcurrentDetect (race-free concurrent access)
  - bench-baseline.txt for regression tracking

affects:
  - graph/louvain_state.go
  - graph/leiden_state.go
  - graph/louvain.go
  - graph/leiden.go
  - graph/benchmark_test.go
  - bench-baseline.txt

tech-stack:
  added:
    - slices package (Go stdlib, replaces O(n^2) insertion sorts)
    - sync.Pool (already in state files; now properly wired)
    - golang.org/x/perf/cmd/benchstat (dev tooling)
  patterns:
    - acquire/reset/release pool pattern
    - dirty-list map cleanup (O(k) instead of full clear)
    - single-pass neighbor weight accumulation (eliminates inner WeightToComm loop)

key-files:
  created:
    - graph/benchmark_test.go: BenchmarkLouvain10K, BenchmarkLeiden10K, TestConcurrentDetect, generateBA
    - bench-baseline.txt: benchstat baseline (5-run counts) for regression tracking
  modified:
    - graph/louvain_state.go: slices.Sort replaces insertion sorts; rand.New(src) in reset
    - graph/leiden_state.go: slices.Sort replaces insertion sorts; rand.New(src) in reset
    - graph/louvain.go: acquireLouvainState/releaseLouvainState, single-pass neighborBuf accumulation, slices.Sort, bestSuperPartition deep copy
    - graph/leiden.go: acquireLeidenState/releaseLeidenState, slices.Sort in refinePartition, bestSuperPartition deep copy

decisions:
  - rand.New(src) in reset() instead of st.rng.Seed(): ensures identical RNG sequence to original newXxxState; Seed() skips internal state initialization causing different shuffle results
  - bestSuperPartition deep copy: pool reuse means state.partition is cleared on reset; pointer sharing silently destroys saved best results
  - single-pass accumulation: replaces per-candidate WeightToComm (O(degree * candidates)) with one neighbor iteration O(degree); 2x speedup
  - slices.Sort replaces all insertion sorts: insertion sort is O(n^2) on 10K nodes; slices.Sort is O(n log n)

metrics:
  duration: 45min
  completed: 2026-03-29
  tasks: 2
  files: 6

performance-results:
  BenchmarkLouvain10K: ~50ms/op (target < 100ms) - PASS
  BenchmarkLeiden10K: ~56ms/op (target < 100ms) - PASS
  race-detector: PASS (zero data races)
  TestConcurrentDetect: PASS
---

# Phase 04 Plan 02: sync.Pool + Benchmarks Summary

sync.Pool state reuse with dirty-list neighbor accumulation for Louvain/Leiden; 10K-node benchmarks at ~50ms/op (< 100ms target); concurrent safety verified under -race.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | sync.Pool + neighborBuf dirty-list for both state structs | 4d5b0d8 | louvain_state.go, leiden_state.go, louvain.go, leiden.go |
| 2 | 10K benchmarks, concurrent safety test, benchstat baseline | 4a85dad | benchmark_test.go, bench-baseline.txt, louvain.go, leiden.go, louvain_state.go, leiden_state.go |

## Verification Results

- `go test ./graph/... -count=1`: PASS
- `go test -race ./graph/... -count=1`: PASS (zero races)
- `BenchmarkLouvain10K`: ~50ms/op (target < 100ms) — PASS
- `BenchmarkLeiden10K`: ~56ms/op (target < 100ms) — PASS
- `TestConcurrentDetect` under -race: PASS
- `bench-baseline.txt`: exists, benchstat shows geomean 52.6ms

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed bestSuperPartition pointer sharing under pool reuse**
- **Found during:** Task 1 (test failures after pool integration)
- **Issue:** Original code did `bestSuperPartition = superPartition` which saved a pointer to `state.partition`. When `state.reset()` is called on the next outer loop iteration, `clear(st.partition)` destroys the saved data. Tests failed with Q=0, communities=1.
- **Fix:** Deep copy partition before saving: `bestSuperPartition = make(map[NodeID]int, len(superPartition))` + copy loop. Applied to both louvain.go and leiden.go.
- **Files modified:** graph/louvain.go, graph/leiden.go
- **Commit:** 4d5b0d8

**2. [Rule 1 - Bug] Fixed RNG seeding in reset() producing wrong shuffle sequence**
- **Found during:** Task 1 (NMI accuracy tests failing with seed-dependent results)
- **Issue:** `st.rng.Seed(src.Int63())` skips internal `rand.Source` state setup vs. `rand.New(src)` in original code. Tests getting NMI=0.67 instead of required >=0.70.
- **Fix:** Replace `st.rng.Seed(...)` with `st.rng = rand.New(src)` in both state reset methods.
- **Files modified:** graph/louvain_state.go, graph/leiden_state.go
- **Commit:** 4d5b0d8

**3. [Rule 2 - Performance] Replaced O(n^2) insertion sorts with slices.Sort**
- **Found during:** Task 2 (benchmarks exceeded 100ms target at ~240ms)
- **Issue:** All sort operations used insertion sort with "small N" comments; at 10K nodes these become O(n^2) bottlenecks.
- **Fix:** Replaced all hot-path sorts with `slices.Sort` (O(n log n)). Files: louvain.go, leiden.go, louvain_state.go, leiden_state.go.
- **Commit:** 4a85dad

**4. [Rule 2 - Performance] Single-pass neighbor weight accumulation in phase1**
- **Found during:** Task 2 (profiling showed WeightToComm/mapaccess dominated after sort fixes)
- **Issue:** phase1 called `deltaQ` per candidate community, each scanning all neighbors O(degree × candidates). At 10K nodes this is the dominant cost.
- **Fix:** Single neighbor pass accumulates edge weights by community into `neighborBuf`; inline ΔQ computation uses precomputed values. 2x speedup (from ~105ms to ~50ms).
- **Files modified:** graph/louvain.go
- **Commit:** 4a85dad

## Known Stubs

None.

## Self-Check: PASS
