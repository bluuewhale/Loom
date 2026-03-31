---
phase: 09-edge-cases-and-hardening
plan: "01"
subsystem: graph/ego_splitting
tags: [edge-cases, hardening, sentinel-error, ego-splitting]
requirements: [EGO-12, EGO-13, EGO-14]

dependency_graph:
  requires: []
  provides: [ErrEmptyGraph sentinel, empty-graph guard in Detect, EGO-12/13/14 test coverage]
  affects: [graph/ego_splitting.go, graph/ego_splitting_test.go]

tech_stack:
  added: []
  patterns: [TDD red-green, sentinel error pattern matching detector.go]

key_files:
  created: []
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go

decisions:
  - "ErrEmptyGraph guard placed after IsDirected check — same pattern as ErrDirectedNotSupported"
  - "TDD RED used a temporary inline test (removed before Task 1 commit) — keeps commit history clean"
  - "TestEgoSplittingDetector_Detect_StarTopology asserts persona count <= degree(center), not == 1; Louvain assigns each disconnected leaf to its own community so center gets 5 personas — bounded not panic"

metrics:
  duration: "~3 minutes"
  completed_date: "2026-03-30"
  tasks_completed: 2
  files_modified: 2
---

# Phase 09 Plan 01: Edge Cases and Hardening Summary

ErrEmptyGraph sentinel + empty-graph guard added to Detect; four edge-case tests cover isolated nodes (EGO-12), star topology (EGO-13), and empty graph (EGO-14).

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add ErrEmptyGraph sentinel and empty-graph guard in Detect | 44979fb | graph/ego_splitting.go |
| 2 | Add edge-case tests for EGO-12, EGO-13, EGO-14 | db2a3d3 | graph/ego_splitting_test.go |

## What Was Built

**Task 1 — `graph/ego_splitting.go`:**
- Exported `ErrEmptyGraph = errors.New("ego splitting: empty graph")` sentinel (line 9)
- Added `if g.NodeCount() == 0` guard immediately after the directed-graph guard in `Detect` (line 66-68)
- `Detect` now returns `OverlappingCommunityResult{}, ErrEmptyGraph` on empty input — no panic, defined contract

**Task 2 — `graph/ego_splitting_test.go`:**
- Added `makeStar(n int) *Graph` helper — center node 0 connected to leaves 1..n
- `TestEgoSplittingDetector_Detect_EmptyGraph` — EGO-14: verifies ErrEmptyGraph returned, Communities and NodeCommunities both nil
- `TestEgoSplittingDetector_Detect_IsolatedNodes` — EGO-12: isolated node 2 appears in NodeCommunities with exactly 1 community membership; all 3 nodes present
- `TestBuildPersonaGraph_IsolatedNode` — EGO-12 unit-level: isolated node 2 gets persona under community key 0 in personaOf and inverseMap
- `TestEgoSplittingDetector_Detect_StarTopology` — EGO-13: no panic on star(5), all 6 nodes in NodeCommunities, center persona count <= degree(5)

## Verification Results

```
go test ./graph/... -count=1   →   ok   github.com/bluuewhale/loom/graph   9.166s
go build ./graph/...           →   exit 0
grep -c "ErrEmptyGraph" graph/ego_splitting.go   →   3
```

## Decisions Made

1. **ErrEmptyGraph guard placement:** After `IsDirected` check, before `buildPersonaGraph` call — mirrors the directed-graph guard pattern already established in detector.go.
2. **Star topology assertion:** `len(centerComms) <= degree(center)` rather than `== 1`. The ego-net of a star center is 5 disconnected leaves; Louvain assigns each singleton to its own community, yielding 5 personas. The invariant that matters (EGO-13) is no panic and no unbounded growth, not a specific community count.
3. **TDD approach:** Wrote a temporary RED test (`TestEgoSplittingDetector_Detect_EmptyGraph_RED`) to verify build failure before implementation, then removed it from the test file before committing. Keeps commit history clean with no noise tests.

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None.

## Self-Check: PASSED
