---
phase: 14
plan: 01
subsystem: graph
tags: [warm-start, benchmarks, alloc-savings, race-stability]
dependency_graph:
  requires: [louvain_state.go, leiden_state.go, ego_splitting.go, benchmark_test.go]
  provides: [TestEgoSplittingUpdateAllocSavings, BenchmarkEgoSplittingUpdate, BenchmarkEgoSplittingDetect1K]
  affects: [graph/benchmark_test.go, graph/ego_splitting_test.go]
tech_stack:
  added: []
  patterns: [build-tag race detection, medianSpeedup helper, package-level benchmark state]
key_files:
  created:
    - graph/race_on_test.go
    - graph/race_off_test.go
  modified:
    - graph/benchmark_test.go
    - graph/ego_splitting_test.go
    - .planning/phases/14-reset-warm-start-commstr-delta-patch/14-01-PLAN.md
decisions:
  - raceEnabled build-tag approach chosen over testing.Race() (not available in Go stdlib) for -race skip guards
  - medianSpeedup helper (3 samples) chosen over single sample for warm-start speedup tests to eliminate scheduling noise
  - Package-level init() vars (bench1KPostDelta, bench1KPrior, bench1KDelta) chosen over closure-local setup to avoid O(N) calibration overhead inside testing.Benchmark loops
  - Dedicated BenchmarkEgoSplittingDetect1K extracted rather than inline closure — eliminates graph rebuild inside b.N loop (was causing 120s timeout)
metrics:
  duration: 21min
  completed: 2026-03-31
  tasks_completed: 2
  files_modified: 4
  files_created: 3
---

# Phase 14 Plan 01: Reset Warm-Start commStr Delta Patch — Summary

**One-liner:** Alloc-savings benchmark suite for `Update()` vs `Detect()` with median-sampled warm-start speedup guards and race-detector skip infrastructure.

## What Was Done

### Task 1: commStr warm-start correctness audit

Audited `louvain_state.go` and `leiden_state.go` `reset()` warm-start paths. Both correctly:
1. Call `clear(st.commStr)` at the top to purge any pool-reused entries
2. Compact partition to 0-indexed contiguous IDs (Steps 1-3)
3. Rebuild `commStr` from `g.Strength(n)` after the remap (Step 4)

No bug found. The `commStr` delta path is correct. All existing tests pass.

### Task 2: TestEgoSplittingUpdateAllocSavings + supporting infrastructure

Added three things:

**`BenchmarkEgoSplittingUpdate`** — measures `Update()` allocs on a 1K BA graph with a single-node delta (attach to min-degree node, ~0.7% affected). Uses shared `bench1KPostDelta` / `bench1KPrior` / `bench1KDelta` package vars.

**`BenchmarkEgoSplittingDetect1K`** — companion cold `Detect()` benchmark on the same post-delta graph. Named function (not inline closure) so `testing.Benchmark` calibration doesn't rebuild the graph on every iteration.

**`TestEgoSplittingUpdateAllocSavings`** — asserts `detect_allocs / update_allocs >= 2.0`. Empirical result: ~4-5x on Apple M4 (Update=~44K allocs/op, Detect=~198K allocs/op).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] TestLeidenWarmStartSpeedup flaky under -race**

- **Found during:** Task 1 — full `go test ./graph/ -race` run
- **Issue:** `TestLeidenWarmStartSpeedup` and `TestLouvainWarmStartSpeedup` use `testing.Benchmark` which only runs a 1s wall-clock budget. Under the race detector (~3x overhead), each iteration is slower, fewer samples are collected, and scheduler noise dominates — producing ratios from 0.99x to 1.42x on identical code.
- **Fix 1:** Added `race_on_test.go` / `race_off_test.go` build-tag files providing `raceEnabled bool` constant; both tests now skip under `-race` with a clear explanation.
- **Fix 2:** Replaced single `testing.Benchmark` call with `medianSpeedup` helper (3 samples, insertion-sort median) to guard against scheduling noise even without `-race`.
- **Files modified:** `graph/benchmark_test.go`, `graph/race_on_test.go` (new), `graph/race_off_test.go` (new)
- **Commit:** b3d6bbf

**2. [Rule 1 - Bug] TestEgoSplittingUpdateAllocSavings timed out at 120s**

- **Found during:** Task 2 — first implementation used inline closure for `detectResult`
- **Issue:** Inline closure rebuilt the 1K graph and found the min-degree node on every call including `testing.Benchmark` calibration iterations. Cold `Detect()` takes ~1.4s/op; calibration tried N=204 → ~280s elapsed → timeout.
- **Fix:** Extracted `BenchmarkEgoSplittingDetect1K` as a named package-level function; moved graph/prior construction to package-level `init()` vars (`bench1KPostDelta`, `bench1KPrior`, `bench1KDelta`). Setup cost is now O(1) per `b.N` iteration.
- **Files modified:** `graph/ego_splitting_test.go`
- **Commit:** b3d6bbf

## Known Stubs

None — all data is wired from real benchmark measurements.

## Self-Check: PASSED

- FOUND: graph/race_on_test.go
- FOUND: graph/race_off_test.go
- FOUND: .planning/phases/14-reset-warm-start-commstr-delta-patch/14-01-SUMMARY.md
- FOUND: commit 685738c (plan file)
- FOUND: commit 6d7f399 (initial benchmark addition)
- FOUND: commit b3d6bbf (stabilization + race fixes)
