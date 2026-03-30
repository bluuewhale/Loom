---
phase: 05-warm-start
plan: 01
subsystem: graph
tags: [warm-start, community-detection, louvain, leiden, options-api]
dependency_graph:
  requires: []
  provides: [InitialPartition field in LouvainOptions/LeidenOptions, warm-seed reset() logic, firstPass guard in Detect loops]
  affects: [graph/detector.go, graph/louvain_state.go, graph/leiden_state.go, graph/louvain.go, graph/leiden.go]
tech_stack:
  added: []
  patterns: [firstPass boolean guard, warm-seed partition seeding with compact remap, commStr rebuild from current graph strengths]
key_files:
  created: []
  modified:
    - graph/detector.go
    - graph/louvain_state.go
    - graph/leiden_state.go
    - graph/louvain.go
    - graph/leiden.go
decisions:
  - InitialPartition passed as reset() parameter, not stored as state field — preserves pool safety (Pitfall 5)
  - firstPass guard ensures warm seed applied only on original graph; supergraph passes always cold-reset (supergraph NodeIDs differ from original)
  - New nodes absent from prior partition get fresh singleton community IDs offset past max prior comm ID
  - commStr rebuilt from current g.Strength() — never copied from prior run
metrics:
  duration: ~15min
  completed: 2026-03-30
  tasks: 2
  files_modified: 5
---

# Phase 05 Plan 01: Warm-Start API Surface and Core Seeding Logic Summary

**One-liner:** InitialPartition field on LouvainOptions/LeidenOptions seeds the phase-1 local-move loop from a prior partition for faster convergence on incrementally updated graphs; nil preserves existing cold-start behavior with zero breaking changes.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | Add InitialPartition to Options structs and update reset() signatures with warm-seed logic | e43ee6b | graph/detector.go, graph/louvain_state.go, graph/leiden_state.go |
| 2 | Add firstPass guard to Louvain and Leiden Detect loops | 70dab51 | graph/louvain.go, graph/leiden.go |

## What Was Built

### graph/detector.go

Added `InitialPartition map[NodeID]int` field (nil = cold start) to both `LouvainOptions` and `LeidenOptions`. No changes to `CommunityResult`.

### graph/louvain_state.go and graph/leiden_state.go

Changed `reset()` signatures from `(g *Graph, seed int64)` to `(g *Graph, seed int64, initialPartition map[NodeID]int)`.

Warm-seed logic when `initialPartition != nil`:
1. Find `maxCommID` in prior partition to compute `nextNewComm` offset for new nodes
2. Assign each node from `initialPartition`; nodes absent get fresh singleton IDs
3. Compact all community IDs to 0-indexed contiguous via deterministic remap (nodes sorted)
4. Rebuild `commStr` from current `g.Strength()` — never carried from prior run

`acquireLouvainState` and `acquireLeidenState` updated to pass `nil` (cold acquire, no behavior change).

### graph/louvain.go and graph/leiden.go

Added `firstPass := true` before each Detect loop. First iteration calls `state.reset(..., d.opts.InitialPartition)`; subsequent iterations call `state.reset(..., nil)`. This ensures InitialPartition is applied only on the original graph — supergraph passes always cold-reset because supergraph NodeIDs don't correspond to original NodeIDs.

## Decisions Made

- **InitialPartition as reset() parameter only:** Not stored as a struct field on louvainState/leidenState. This preserves pool safety — pooled state objects must not retain caller-owned references between Detect calls.
- **firstPass guard:** Warm seed applies only on the first supergraph pass (original graph). Subsequent passes (compressed supergraph) always use nil because supergraph NodeIDs are synthetic and don't map to prior-run NodeIDs.
- **commStr rebuilt from g.Strength():** The commStr cache must reflect the current graph's edge weights, not the prior run's weights. Warm start only seeds the community assignment, not cached metrics.
- **New nodes get singleton offset:** Nodes absent from `initialPartition` are assigned IDs starting at `maxCommID+1` before the compaction step — avoids collisions with seeded communities.

## Deviations from Plan

None — plan executed exactly as written.

## Verification

Build verification (`go build ./graph/...`) could not be executed: Go toolchain not available in the execution environment. All acceptance criteria verified via grep:

- `grep -c "InitialPartition map" graph/detector.go` → 2 (one per options struct)
- `grep "func.*louvainState.*reset"` → contains `initialPartition` parameter
- `grep "func.*leidenState.*reset"` → contains `initialPartition` parameter
- `grep -c "maxCommID" graph/louvain_state.go` → 3 matches (warm-seed logic present)
- `grep -c "maxCommID" graph/leiden_state.go` → 3 matches (warm-seed logic present)
- `grep -c "firstPass" graph/louvain.go` → 3 (declaration + if + set false)
- `grep -c "firstPass" graph/leiden.go` → 3 (declaration + if + set false)
- `grep -c "d.opts.InitialPartition" graph/louvain.go` → 1
- `grep -c "d.opts.InitialPartition" graph/leiden.go` → 1

## Known Stubs

None.

## Self-Check: PASSED

- graph/detector.go modified: FOUND
- graph/louvain_state.go modified: FOUND
- graph/leiden_state.go modified: FOUND
- graph/louvain.go modified: FOUND
- graph/leiden.go modified: FOUND
- Commit e43ee6b: FOUND
- Commit 70dab51: FOUND
