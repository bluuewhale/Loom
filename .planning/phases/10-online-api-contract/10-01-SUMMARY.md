---
phase: 10-online-api-contract
plan: "01"
subsystem: graph
tags: [online-detection, api-contract, ego-splitting, incremental]
dependency_graph:
  requires: [graph/ego_splitting.go, graph/detector.go]
  provides: [GraphDelta type, OnlineOverlappingCommunityDetector interface, NewOnlineEgoSplitting constructor, Update method]
  affects: [graph/ego_splitting.go, graph/ego_splitting_test.go]
tech_stack:
  added: []
  patterns: [compile-time interface check, zero-alloc fast path, TDD red-green]
key_files:
  created: []
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go
decisions:
  - "Update() empty-delta fast-path returns prior by value with 0 allocs (no deep-copy — callers must not mutate prior result)"
  - "NewOnlineEgoSplitting returns *egoSplittingDetector (same struct as NewEgoSplitting) — no new struct needed, satisfies both interfaces"
  - "Non-empty delta falls back to full Detect() call in Phase 10; Phase 11 replaces with incremental recomputation"
  - "GraphDelta placed after EgoSplittingOptions in file layout — clear visual grouping of online API types together"
metrics:
  duration: 1min
  completed_date: "2026-03-31"
  tasks_completed: 1
  files_modified: 2
requirements_satisfied: [ONLINE-01, ONLINE-02, ONLINE-03, ONLINE-04]
---

# Phase 10 Plan 01: Online API Contract Summary

**One-liner:** `GraphDelta` type + `OnlineOverlappingCommunityDetector` interface + `Update()` method with directed-graph guard and 0-alloc empty-delta fast path.

## What Was Built

Extended `graph/ego_splitting.go` with the public API surface for online/incremental ego-splitting updates:

- **`GraphDelta` struct** — `AddedNodes []NodeID`, `AddedEdges []Edge` (ONLINE-01)
- **`OnlineOverlappingCommunityDetector` interface** — embeds `OverlappingCommunityDetector` and adds `Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)` (ONLINE-02)
- **`NewOnlineEgoSplitting(opts EgoSplittingOptions) OnlineOverlappingCommunityDetector`** — constructor with same nil-defaults as `NewEgoSplitting`
- **`(*egoSplittingDetector).Update()`** — three behaviors:
  1. Directed graph → `ErrDirectedNotSupported` (ONLINE-04)
  2. Empty delta → return `prior` unchanged, 0 allocs/op (ONLINE-03)
  3. Non-empty delta → fall back to `d.Detect(g)` (Phase 11 placeholder)

Extended `graph/ego_splitting_test.go` with:

- Compile-time check: `var _ OnlineOverlappingCommunityDetector = (*egoSplittingDetector)(nil)`
- `TestNewOnlineEgoSplitting_ReturnsInterface`
- `TestEgoSplittingDetector_Update_DirectedGraphError`
- `TestEgoSplittingDetector_Update_EmptyDelta_ReturnsPrior`
- `TestEgoSplittingDetector_Update_NonEmptyDelta_Placeholder`
- `BenchmarkUpdate_EmptyDelta` — confirmed 0 allocs/op, 1.618 ns/op

## Commits

| Task | Commit | Description |
|------|--------|-------------|
| RED  | 077cb9f | test(10-01): add failing tests for ONLINE-01 through ONLINE-04 |
| GREEN | f9f553c | feat(10-01): add GraphDelta, OnlineOverlappingCommunityDetector, NewOnlineEgoSplitting, Update() |

## Verification Results

```
go build ./graph/...            PASS
go test ./graph/... -count=1    PASS (all tests, 8.338s)
go vet ./graph/...              PASS
BenchmarkUpdate_EmptyDelta      789814467 ops — 1.618 ns/op — 0 B/op — 0 allocs/op
```

## Decisions Made

1. **`Update()` empty-delta returns prior by value** — no deep-copy. Callers that hold a reference to `prior.Communities` or `prior.NodeCommunities` must not mutate those slices; this is consistent with all other `Detect`-family return values in the library.
2. **Reuse `*egoSplittingDetector`** — `NewOnlineEgoSplitting` returns the same struct as `NewEgoSplitting`. No new struct was created because the only new behaviour is the `Update()` method which was added to the existing receiver. This keeps the type hierarchy flat.
3. **Non-empty delta falls back to `Detect()`** — Phase 10 goal is API contract only. The incremental recomputation (ONLINE-05, ONLINE-06, ONLINE-07) is Phase 11 scope.

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

- `Update()` non-empty-delta path calls `d.Detect(g)` with a `// TODO(Phase 11): replace with incremental recomputation.` comment. This is intentional per the plan spec — Phase 11 will implement the incremental path (ONLINE-05, ONLINE-06, ONLINE-07).

## Self-Check: PASSED

- `graph/ego_splitting.go` — modified, exists
- `graph/ego_splitting_test.go` — modified, exists
- Commit 077cb9f — exists (`git log` confirmed)
- Commit f9f553c — exists (`git log` confirmed)
- `go test ./graph/... -count=1` — PASS
- `BenchmarkUpdate_EmptyDelta` — 0 allocs/op confirmed
