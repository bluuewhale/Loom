---
phase: 08-full-detect-pipeline-accuracy-performance
plan: 01
subsystem: graph
tags: [overlapping-community-detection, ego-splitting, omega-index, accuracy-metric]
dependency_graph:
  requires: [07-persona-graph-infrastructure]
  provides: [OmegaIndex, EgoSplittingDetector.Detect]
  affects: [graph/ego_splitting.go, graph/omega.go]
tech_stack:
  added: []
  patterns: [pair-counting accuracy metric, sparse-to-contiguous community index compaction]
key_files:
  created:
    - graph/omega.go
    - graph/omega_test.go
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go
decisions:
  - "OmegaIndex precomputes resultMembership and gtMembership maps for O(C) per-pair lookup instead of O(K) community scan"
  - "Detect() Step 4 deduplicates community IDs per node — mapPersonasToOriginal can emit duplicates when multiple personas of the same node land in the same global community"
  - "commRemap compact pass removes sparse community ID gaps so Communities[i] has no nil holes and NodeCommunities indices stay consistent"
  - "ErrNotImplemented test (Test 4) replaced with ErrDirectedNotSupported test + triangle smoke test — Detect no longer stubs"
metrics:
  duration: ~20min
  completed: "2026-03-30T08:06:15Z"
  tasks_completed: 2
  files_changed: 4
---

# Phase 08 Plan 01: Full Detect Pipeline + OmegaIndex Summary

OmegaIndex accuracy metric (Collins & Dent 1988 adjusted pair-counting) and wired EgoSplittingDetector.Detect() pipeline replacing ErrNotImplemented stub.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Implement OmegaIndex in graph/omega.go | 1321f5d | graph/omega.go, graph/omega_test.go |
| 2 | Wire Detect() pipeline in ego_splitting.go | 26e449e | graph/ego_splitting.go, graph/ego_splitting_test.go |

## What Was Built

**Task 1 — OmegaIndex (`graph/omega.go`)**

`OmegaIndex(result OverlappingCommunityResult, groundTruth [][]NodeID) float64` implements Collins & Dent (1988) adjusted pair-counting:

1. Collect all unique nodes from both inputs into a sorted slice.
2. Precompute `resultMembership` (node → set of community indices) and `gtMembership` from ground truth.
3. For every unordered pair (u, v): count shared memberships `tResult` and `tGT` via set intersection; track `agree`, `freqResult[tResult]`, `freqGT[tGT]`.
4. `observed = agree / totalPairs`; `expected = Σ_k freqResult[k]*freqGT[k] / totalPairs²`
5. `omega = (observed − expected) / (1 − expected)`, clamped to [0, 1].

Unit tests: identical partitions → 1.0; completely different → < 0.5; empty → 0.0; two-node identical → 1.0; in-range property test across 4 scenarios.

**Task 2 — Detect() pipeline (`graph/ego_splitting.go`)**

Replaced the `ErrNotImplemented` stub with the full Ego Splitting pipeline:

1. Directed-graph guard → `ErrDirectedNotSupported`
2. `buildPersonaGraph(g, d.opts.LocalDetector)` — Algorithms 1+2
3. `d.opts.GlobalDetector.Detect(personaGraph)` — global community detection
4. `mapPersonasToOriginal(globalResult.Partition, inverseMap)` — Algorithm 3
5. Deduplicate community IDs per node (in-place, no alloc when no dups)
6. Build `communities [][]NodeID` from `nodeCommunities`
7. Compact sparse community IDs via `commRemap` → contiguous indices; remap `NodeCommunities` to match

Updated Test 4: `TestEgoSplittingDetector_Detect_ReturnsErrNotImplemented` → `TestEgoSplittingDetector_Detect_DirectedGraphError` + `TestEgoSplittingDetector_Detect_Triangle`.

## Verification Results

```
go build ./...          — OK
go vet ./graph/         — OK
go test ./graph/ -count=1 — ok github.com/bluuewhale/loom/graph 5.785s (all pass)
```

- `TestOmegaIndex_IdenticalPartitions` — PASS
- `TestOmegaIndex_CompletelyDifferent` — PASS
- `TestOmegaIndex_EmptyInput` — PASS
- `TestOmegaIndex_TwoNodes` — PASS
- `TestOmegaIndex_ReturnsInRange` — PASS (4 subtests)
- `TestEgoSplittingDetector_Detect_DirectedGraphError` — PASS
- `TestEgoSplittingDetector_Detect_Triangle` — PASS
- `TestPersonaGraphKarateClub_OverlappingMembership` — PASS (67 persona nodes)
- `TestPersonaGraphKarateClub_AllNodesAccountedFor` — PASS

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None — `ErrNotImplemented` stub replaced; `Detect()` runs the full pipeline.

## Self-Check: PASSED
