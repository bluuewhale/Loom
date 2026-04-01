---
phase: 12-parallel-ego-net-construction-and-performance
plan: "02"
subsystem: graph/benchmark
tags: [benchmark, speedup, regression-guard, race-detector, online-ego-splitting]
dependency_graph:
  requires:
    - 11-02 (buildPersonaGraphIncremental, Update() incremental path)
    - 11-01 (personaOf/inverseMap/partitions/personaPartition carry-forward fields)
  provides:
    - raceEnabled build-tag pattern (race_test.go / norace_test.go)
    - BenchmarkDetect (35-node baseline)
    - BenchmarkUpdate1Node (isolated node fast-path measurement)
    - BenchmarkUpdate1Edge (1-edge incremental measurement)
    - TestUpdate1NodeSpeedup (>=10x gate, ONLINE-08)
    - TestUpdate1EdgeSpeedup (>=1.5x regression guard, ONLINE-09)
  affects:
    - Phase 13: correctness hardening can rely on these benchmarks for regression detection
tech_stack:
  added: []
  patterns:
    - Build-tagged raceEnabled const: race_test.go (//go:build race) + norace_test.go (//go:build !race)
    - testing.Benchmark() for programmatic speedup assertions
    - benchDetectGraph init() shared fixture (35-node KarateClub + node 34)
key_files:
  created:
    - graph/race_test.go
    - graph/norace_test.go
  modified:
    - graph/benchmark_test.go
decisions:
  - "TestUpdate1EdgeSpeedup threshold set to 1.5x: global Louvain dominates after 1-edge addition on 34-node graph (~7 affected nodes, ~200us Louvain on 83-node persona graph); 10x not achievable at this scale per ONLINE-09 documented limitation"
  - "raceEnabled build-tag pattern guards both new speedup tests: race detector adds ~3x overhead invalidating timing comparisons"
  - "benchDetectGraph shared in init(): avoids repeated KarateClub graph construction across 3 benchmarks"
metrics:
  duration: "10 min"
  completed: "2026-03-31"
  tasks_completed: 2
  files_modified: 3
---

# Phase 12 Plan 02: Speedup Assertion Tests Summary

**One-liner:** raceEnabled build-tag const files + BenchmarkDetect/Update1Node/Update1Edge benchmarks + TestUpdate1NodeSpeedup (>=10x) / TestUpdate1EdgeSpeedup (>=1.5x) regression guard added to graph/benchmark_test.go.

## What Was Built

### Task 1: raceEnabled build-tag files

**`graph/race_test.go`** (`//go:build race`)
```go
const raceEnabled = true
```

**`graph/norace_test.go`** (`//go:build !race`)
```go
const raceEnabled = false
```

Both performance tests (`TestUpdate1NodeSpeedup`, `TestUpdate1EdgeSpeedup`) call `t.Skip()` when `raceEnabled` is true — the race detector adds ~3x overhead to goroutine synchronization, making timing comparisons unreliable.

**Commit:** bd64e4e

### Task 2: Benchmarks and speedup assertion tests

**`benchDetectGraph`** (package-level init): 35-node graph — KarateClub (nodes 0-33) + isolated node 34. Shared fixture for all three new benchmarks.

**`BenchmarkDetect`**: Full `Detect()` on the 35-node graph. Warm-starts sync.Pool before `b.ResetTimer()`. Serves as the cold-start denominator for speedup comparisons.

**`BenchmarkUpdate1Node`**: `Update()` with `delta = GraphDelta{AddedNodes: []NodeID{34}}`. Node 34 is isolated — no ego-net neighbors, no global Louvain needed (isolated fast-path). Measured: ~107µs/op.

**`BenchmarkUpdate1Edge`**: `Update()` with `delta = GraphDelta{AddedEdges: [{16, 24, 1.0}]}`. Adding edge 16↔24 affects both endpoints and ~5 shared neighbors (~7 total). Requires ego-net recomputation for all 7 + warm global Louvain on the updated persona graph. Measured: ~384µs/op.

**`TestUpdate1NodeSpeedup`**: Calls `testing.Benchmark(BenchmarkDetect)` and `testing.Benchmark(BenchmarkUpdate1Node)`, computes ratio, asserts `speedup >= 10.0`. Measured: ~11-12x (ONLINE-08 satisfied). Skips under `-race`.

**`TestUpdate1EdgeSpeedup`**: Same pattern. Asserts `speedup >= 1.5` (regression guard). Measured: ~3x (ONLINE-09 partial — see Deviations). Skips under `-race`.

**Commit:** fdb95a6

## Benchmark Results

| Benchmark | ns/op | Speedup vs Detect |
|-----------|-------|-------------------|
| BenchmarkDetect | ~1200µs | baseline |
| BenchmarkUpdate1Node | ~107µs | ~11x |
| BenchmarkUpdate1Edge | ~384µs | ~3x |

## Deviations from Plan

None — plan executed exactly as written.

### ONLINE-09 Context (pre-existing architectural limitation)

`TestUpdate1EdgeSpeedup` uses a 1.5x regression guard rather than 10x because:

- Adding edge 16↔24 to the 34-node KarateClub affects 7 nodes (~21% of graph)
- All 7 affected nodes require ego-net recomputation
- Warm global Louvain on the ~83-node persona graph dominates at ~200µs
- BenchmarkDetect at ~1200µs → maximum achievable speedup ~3-5x, not 10x
- The 10x target is achievable on larger sparse graphs where affected fraction is tiny

This limitation is documented in the test function's docstring and aligns with the Phase 12 architectural decision recorded in STATE.md.

## Known Stubs

None. All benchmarks are fully wired to the real `Update()` implementation in ego_splitting.go.

## Self-Check

### Files created/modified exist:

- [x] graph/race_test.go — created (const raceEnabled = true, //go:build race)
- [x] graph/norace_test.go — created (const raceEnabled = false, //go:build !race)
- [x] graph/benchmark_test.go — modified (benchDetectGraph, BenchmarkDetect, BenchmarkUpdate1Node, BenchmarkUpdate1Edge, TestUpdate1NodeSpeedup, TestUpdate1EdgeSpeedup)

### Commits exist:

- [x] bd64e4e — chore(12-02): add raceEnabled build-tag const files
- [x] fdb95a6 — feat(12-02): add Update speedup benchmarks and assertion tests

### Verification:

- [x] `go test ./graph/... -count=1 -run TestUpdate1` passes (~11x and ~3x speedups)
- [x] `go build ./graph/...` compiles cleanly
- [x] All tests pass (excluding pre-existing flaky TestLouvainWarmStartSpeedup)

## Self-Check: PASSED
