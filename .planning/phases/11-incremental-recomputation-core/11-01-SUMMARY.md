---
phase: 11-incremental-recomputation-core
plan: "01"
subsystem: graph/ego_splitting
tags: [incremental, carry-forward, warm-start, tdd]
dependency_graph:
  requires:
    - 10-01 (GraphDelta, Update() stub, OnlineOverlappingCommunityDetector)
    - 08-01 (Detect() pipeline, buildPersonaGraph, mapPersonasToOriginal)
  provides:
    - carry-forward fields on OverlappingCommunityResult (personaOf, inverseMap, partitions, personaPartition)
    - warmStartedDetector helper (type switch on *louvainDetector/*leidenDetector)
  affects:
    - 11-02 (buildPersonaGraphIncremental will read carry-forward fields from prior result)
tech_stack:
  added: []
  patterns:
    - Unexported carry-forward fields on result struct (zero-value nil = cold-start sentinel)
    - Type switch helper for constructing warm-start detectors without mutation
key_files:
  created: []
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go
decisions:
  - "buildPersonaGraph returns partitions as 4th value — avoids re-building for Detect() caller and also exposes it for the carry-forward field without a separate pass"
  - "warmStartedDetector falls back to d unchanged for unknown types — safe open/closed extension point for future detector implementations"
  - "Carry-forward fields are unexported — external callers cannot depend on intermediate state; Update() accesses them via package-internal access"
metrics:
  duration: "5 min"
  completed: "2026-03-31"
  tasks_completed: 2
  files_modified: 2
---

# Phase 11 Plan 01: Carry-Forward Fields + warmStartedDetector Summary

**One-liner:** Added 4 unexported carry-forward fields (personaOf, inverseMap, partitions, personaPartition) to OverlappingCommunityResult and a warmStartedDetector type-switch helper enabling Plan 11-02's incremental Update() path.

## What Was Built

### Task 1: Carry-Forward Fields on OverlappingCommunityResult

`OverlappingCommunityResult` gained 4 unexported fields populated by `Detect()`:

- `personaOf map[NodeID]map[int]NodeID` — original node → community → PersonaID
- `inverseMap map[NodeID]NodeID` — PersonaID → original NodeID
- `partitions map[NodeID]map[NodeID]int` — ego-net partition per original node
- `personaPartition map[NodeID]int` — persona-level global partition

`buildPersonaGraph` signature extended to return `partitions` as a 4th value (was previously built internally but discarded). `Detect()` now captures `personaOf` and `partitions` from `buildPersonaGraph` and stores all 4 carry-forward fields on the returned result before returning.

A zero-value `OverlappingCommunityResult` has nil carry-forward fields, which `Update()` (Plan 11-02) will use as a cold-start sentinel.

**Commit:** b9f2a12

### Task 2: warmStartedDetector Helper

`warmStartedDetector(d CommunityDetector, partition map[NodeID]int) CommunityDetector` constructs a new detector with identical options but `InitialPartition` set to `partition`. Uses a type switch on `*louvainDetector` / `*leidenDetector`. Falls back to `d` unchanged for unrecognized types. Does not mutate the original detector.

This enables `Update()` to warm-start global detection from the prior `personaPartition` without modifying `d.opts.GlobalDetector`.

**Commit:** f50e233

## Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestDetect_PopulatesCarryForwardFields` | ego_splitting_test.go | All 4 fields non-nil after Detect on KarateClub; 34 entries in personaOf/partitions |
| `TestDetect_CarryForwardNilFallback` | ego_splitting_test.go | Zero-value result has nil carry-forward fields |
| `TestWarmStartedDetector_Louvain` | ego_splitting_test.go | Options preserved + InitialPartition set |
| `TestWarmStartedDetector_Leiden` | ego_splitting_test.go | Options preserved + InitialPartition set |
| `TestWarmStartedDetector_NilPartition` | ego_splitting_test.go | nil partition → nil InitialPartition |
| `TestWarmStartedDetector_DoesNotMutateOriginal` | ego_splitting_test.go | Original detector unchanged after call |

## Deviations from Plan

### Pre-existing flaky test

`TestLeidenWarmStartSpeedup` fails consistently on this machine (warm-start speedup 1.15–1.19x vs 1.2x threshold) — pre-existing issue from Phase 05, present on the feature branch before any Phase 11 changes. Not caused by this plan. Logged as out-of-scope.

All other tests pass including `-race`.

## Known Stubs

None. Carry-forward fields are fully populated by `Detect()`. `warmStartedDetector` is a complete implementation. The `Update()` method (still falling back to `d.Detect(g)`) is an intentional Phase 10 stub that Plan 11-02 will replace.

## Self-Check

### Files created/modified exist:

- [x] graph/ego_splitting.go — modified
- [x] graph/ego_splitting_test.go — modified
- [x] .planning/phases/11-incremental-recomputation-core/11-01-SUMMARY.md — created

### Commits exist:

- [x] b9f2a12 — carry-forward fields
- [x] f50e233 — warmStartedDetector

## Self-Check: PASSED
