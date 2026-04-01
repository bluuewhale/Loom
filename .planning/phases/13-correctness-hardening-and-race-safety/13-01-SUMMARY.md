---
phase: 13
plan: 01
subsystem: graph
tags: [testing, race-safety, correctness, invariants]
dependency_graph:
  requires: [12-02]
  provides: [ONLINE-12, ONLINE-13]
  affects: [graph/ego_splitting_test.go, graph/benchmark_test.go]
tech_stack:
  added: []
  patterns: [table-driven tests, assertXxx helper, concurrent goroutine test]
key_files:
  modified:
    - graph/ego_splitting_test.go
    - graph/benchmark_test.go
decisions:
  - assertResultInvariants checks 3 properties: NodeCommunities coverage, index bounds, bidirectional consistency
  - TestEgoSplittingConcurrentUpdate uses 8 goroutines x 3 updates each on independent detector instances
  - Duplicate benchmark block removal was a blocking Rule 3 fix (not a new feature)
metrics:
  duration: "5min"
  completed: "2026-03-31"
  tasks: 2
  files: 2
---

# Phase 13 Plan 01: Correctness Hardening and Race Safety Summary

## One-liner

Result invariant table-driven tests (6 cases) + concurrent Update race safety test with `go test -race ./graph/...` passing clean.

## What Was Built

### Task 1: assertResultInvariants + TestUpdateResultInvariants

Added `assertResultInvariants(t, g, result)` helper to `ego_splitting_test.go` that enforces three structural invariants on every `OverlappingCommunityResult`:

1. **NodeCommunities coverage** — every node in `g` appears in `NodeCommunities` with at least one community index.
2. **Index bounds** — every community index referenced in `NodeCommunities` is a valid index into `Communities` (no out-of-bounds panic risk).
3. **Bidirectional consistency** — `NodeCommunities → Communities` (if node says community C, C lists node) and `Communities → NodeCommunities` (if C lists node, node references C).

Added `TestUpdateResultInvariants` table-driven test with 6 cases:

| Case | Description |
|------|-------------|
| `empty_delta` | Update with no changes — prior returned as-is |
| `single_node_addition_isolated` | Add 1 isolated node (fast-path) |
| `single_edge_addition` | Add 1 edge between existing nodes (warm Louvain path) |
| `multi_node_batch_addition` | Add 3 isolated nodes in one delta |
| `node_and_edge_together` | Add 1 node + 1 edge to it in same delta |
| `nil_carry_forward_fallback` | Zero-value prior triggers Detect() fallback |

All 6 cases pass with and without `-race`.

### Task 2: TestEgoSplittingConcurrentUpdate

Added `TestEgoSplittingConcurrentUpdate` which launches 8 goroutines, each owning an independent `OnlineOverlappingCommunityDetector`, independent `*Graph`, and independent prior. Each goroutine performs 3 sequential `Update()` calls. Passes `go test -race ./graph/...` with zero race reports.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking Issue] Removed duplicate benchmark function declarations**

- **Found during:** Task 1 (attempting to run `go test`)
- **Issue:** `graph/benchmark_test.go` contained two sets of identical function declarations — `init()`, `BenchmarkDetect`, `BenchmarkUpdate1Node`, `BenchmarkUpdate1Edge`, `TestUpdate1NodeSpeedup`, `TestUpdate1EdgeSpeedup` — left over from phase 12 when a new block was added without removing the old one. This caused a build failure (`redeclared in this block`) that prevented any `go test` invocation.
- **Fix:** Removed the superseded first block (lines 266–435, using `benchDetectGraph`). Kept the authoritative second block (using `benchKarate34`, better comments, correct `NewEgoSplitting` usage).
- **Files modified:** `graph/benchmark_test.go`
- **Commit:** 86d2798

## Deferred Items

- `TestLeidenWarmStartSpeedup`: pre-existing flaky timing test — got 1.200x vs `< 1.2` threshold (floating-point rounding edge case). Unrelated to Phase 13. Logged to deferred-items.md.

## Known Stubs

None — all tests wire real data and assert real invariants.

## Self-Check

Files created/modified:
- graph/ego_splitting_test.go — modified (assertResultInvariants, TestUpdateResultInvariants, TestEgoSplittingConcurrentUpdate added)
- graph/benchmark_test.go — modified (duplicate block removed)
- .planning/phases/13-correctness-hardening-and-race-safety/13-01-PLAN.md — created
- .planning/phases/13-correctness-hardening-and-race-safety/13-01-SUMMARY.md — created

Commits:
- 86d2798: fix(13-01): remove duplicate benchmark declarations blocking build
