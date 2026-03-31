---
phase: 07-persona-graph-infrastructure
plan: 01
subsystem: graph
tags: [ego-splitting, persona-graph, overlapping-communities, algorithm]
dependency_graph:
  requires: [graph/graph.go, graph/detector.go, graph/ego_splitting.go (stub from phase 06)]
  provides: [buildEgoNet, buildPersonaGraph, mapPersonasToOriginal helpers]
  affects: [graph/ego_splitting.go, graph/ego_splitting_test.go]
tech_stack:
  added: []
  patterns: [ego-net subgraph extraction, persona ID allocation above maxNodeID, cross-ego-net partition lookup]
key_files:
  created: [graph/ego_splitting.go, graph/ego_splitting_test.go]
  modified: []
decisions:
  - "PersonaIDs allocated from maxNodeID+1 upward — guarantees zero collision with original NodeID space"
  - "Edge wiring uses cross-ego-net lookup: persona of u determined by community of v in G_u, persona of v determined by community of u in G_v (matches Ego Splitting paper Section 2.2)"
  - "Fallback to community 0 when neighbor absent from ego-net partition — handles bridge nodes and isolated neighbors without dropping edges"
metrics:
  duration: 4min
  completed: 2026-03-30
  tasks_completed: 1
  files_created: 2
---

# Phase 7 Plan 1: Persona Graph Infrastructure Summary

**One-liner:** Ego-net extraction and persona graph construction via cross-ego-net community lookup with TotalWeight preservation.

## What Was Built

Three unexported helper functions implementing the core building blocks of the Ego Splitting algorithm (Epasto, Lattanzi, Paes Leme, 2017):

1. **`buildEgoNet(g *Graph, v NodeID) *Graph`** — Algorithm 1: returns the subgraph induced by v's neighbors, never including v itself. Delegates to `g.Subgraph()` with neighbor To-fields.

2. **`buildPersonaGraph(g *Graph, localDetector CommunityDetector)`** — Algorithm 2: for each node v, runs local community detection on `G_v`, allocates one PersonaID per (v, community) pair starting at `maxNodeID+1`, then rewires all edges using cross-ego-net partition lookups. Returns `(personaGraph, personaOf, inverseMap, error)`.

3. **`mapPersonasToOriginal(globalPartition map[NodeID]int, inverseMap map[NodeID]NodeID)`** — Algorithm 3: inverts the global partition on persona nodes back to overlapping community memberships on original nodes.

## Tests Added

6 new tests in `graph/ego_splitting_test.go`:
- `TestBuildEgoNet_Triangle` — ego-net of node 0 contains {1,2} and edge (1,2)
- `TestBuildEgoNet_ExcludesEgoNode` — v never appears in its own ego-net for all nodes
- `TestBuildPersonaGraph_Triangle` — PersonaIDs ≥ 3, TotalWeight preserved
- `TestBuildPersonaGraph_Barbell` — PersonaIDs disjoint from [0,4), TotalWeight preserved
- `TestBuildPersonaGraph_PersonaIDsDisjoint` — explicit collision check
- `TestMapPersonasToOriginal_Bijective` — all personas mapped, all original nodes covered

All 5 existing Phase 06 tests continue to pass.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed cross-ego-net partition lookup for edge wiring**
- **Found during:** Task 1 GREEN phase (barbell test)
- **Issue:** Plan's pseudocode used `c_u = partitions[v][u]` then `personaOf[u][c_u]` — but `personaOf[u]` is keyed by communities from `G_u`, not `G_v`. These are different community numbering spaces. For the barbell bridge edge (2,3): node 3's ego-net has only node 2 (community 0), while node 2's ego-net puts node 3 in community 1. `personaOf[3][1]` doesn't exist → edge was silently dropped → TotalWeight 3 instead of 4.
- **Fix:** Corrected the lookup to match the paper's semantics: persona of u is chosen by the community of v in G_u (`commOfVinGu = partitions[u][v]`), and persona of v is chosen by the community of u in G_v (`commOfUinGv = partitions[v][u]`). Added fallback to community 0 when a node is absent from the partner's ego-net partition (handles bridge nodes with degree-1 neighbors).
- **Files modified:** `graph/ego_splitting.go`
- **Commit:** e1a3ecc

## Known Stubs

- `Detect()` on `egoSplittingDetector` still returns `ErrNotImplemented` — intentional stub; Phase 08 will wire `buildPersonaGraph` into the full algorithm.

## Self-Check: PASSED

- graph/ego_splitting.go: FOUND
- graph/ego_splitting_test.go: FOUND
- Commit e1a3ecc: FOUND
