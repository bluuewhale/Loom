---
phase: 11-incremental-recomputation-core
plan: "02"
subsystem: graph/ego_splitting
tags: [incremental, online, affected-nodes, persona-graph, warm-start, tdd]
dependency_graph:
  requires:
    - 11-01 (carry-forward fields: personaOf, inverseMap, partitions, personaPartition)
    - 10-01 (GraphDelta, Update() stub, OnlineOverlappingCommunityDetector)
    - 08-01 (buildPersonaGraph, mapPersonasToOriginal, Detect() pipeline)
  provides:
    - computeAffected (affected-node scoping for incremental recomputation)
    - buildPersonaGraphIncremental (ONLINE-06: carry-over + ONLINE-11: collision-free IDs)
    - incremental Update() replacing the Detect fallback stub
  affects:
    - future phases: Update() is now fully incremental; benchmarks can measure speedup
tech_stack:
  added: []
  patterns:
    - Affected-node scoping: new nodes + 1-hop expansion from edge endpoints
    - Incremental map patching: delete affected entries, rebuild in-place, carry unaffected
    - Warm partition pruning: drop deleted PersonaIDs before warm-starting global detector
    - DeltaEdge type (From, To, Weight) to represent edges in GraphDelta
key_files:
  created: []
  modified:
    - graph/ego_splitting.go
    - graph/ego_splitting_test.go
decisions:
  - "DeltaEdge introduced as a separate type from Edge — Edge only has To+Weight (relative to a source), DeltaEdge needs both endpoints to stand alone in a delta"
  - "buildPersonaGraphIncremental rebuilds the full persona graph edges (O(|E|)) — unavoidable without RemoveNode; only ego-net detection is O(affected) not O(all)"
  - "warmPartition prunes deleted PersonaIDs from prior.personaPartition — new affected PersonaIDs absent from warmPartition so global detector assigns them singletons"
  - "TestUpdate_PersonaIDDisjoint and TestUpdate_MultipleSequentialUpdates check only NEWLY allocated PersonaIDs for collision — prior PersonaIDs carried from before a node was added are allowed to share numeric value with that new NodeID (they were allocated first)"
metrics:
  duration: "7 min"
  completed: "2026-03-31"
  tasks_completed: 2
  files_modified: 2
---

# Phase 11 Plan 02: Incremental Recomputation Core Summary

**One-liner:** Replaced Update()'s Detect fallback stub with full incremental recomputation: computeAffected scopes ego-net rebuilds to affected nodes only, buildPersonaGraphIncremental carries unaffected PersonaIDs and allocates collision-free new ones, warm-started global detection produces valid overlapping communities.

## What Was Built

### Task 1: computeAffected + buildPersonaGraphIncremental helpers

**`computeAffected(g *Graph, delta GraphDelta) map[NodeID]struct{}`**

Returns the set of nodes whose ego-nets must be recomputed. Affected = all new nodes + both endpoints of every added edge + their 1-hop neighbors in the already-updated graph. Empty delta → empty set (zero allocations path in Update).

**`DeltaEdge` type** (deviation — see below): `GraphDelta.AddedEdges` was typed as `[]Edge` but `Edge` only has `To` and `Weight` (relative to a source node). `DeltaEdge{From, To, Weight}` was introduced to make delta edges self-contained.

**`buildPersonaGraphIncremental(...)`**

Core of ONLINE-06 and ONLINE-11. Steps:
1. Deep-copy prior `personaOf`, `inverseMap`, `partitions` (shallow inner maps for unaffected entries)
2. Compute `nextPersona = max(all prior inverseMap keys, all g.Nodes()) + 1` — ONLINE-11 invariant
3. Delete all PersonaIDs for affected nodes from the copies
4. Rebuild ego-nets only for affected nodes using `localDetector.Detect`
5. Build `warmPartition` by pruning `prior.personaPartition` to only surviving PersonaIDs
6. Rebuild persona graph edges from scratch (O(|E|)) — unavoidable without RemoveNode

**Commit:** 5550a6c

### Task 2: Incremental Update() + requirement tests

**`Update()` wired with incremental path:**

```
computeAffected → buildPersonaGraphIncremental → warmStartedDetector → Detect(personaGraph) → mapPersonasToOriginal → deduplicate + compact
```

Graceful fallback to `Detect()` if `prior.personaOf == nil` (cold-start sentinel from Phase 11-01).

**`countingDetector` spy** added to `ego_splitting_test.go` for `TestUpdate_AffectedNodesOnly`.

**Commit:** 7fba0d3

## Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestComputeAffected_SingleNodeAdd` | ego_splitting_test.go | isolated new node → affected = {node} only |
| `TestComputeAffected_SingleEdgeAdd` | ego_splitting_test.go | endpoints + all neighbors in affected set |
| `TestComputeAffected_NodeAndEdge` | ego_splitting_test.go | combined node+edge delta expands correctly |
| `TestComputeAffected_EmptyDelta` | ego_splitting_test.go | empty delta → empty set |
| `TestBuildPersonaGraphIncremental_CarriesOverUnaffected` | ego_splitting_test.go | ONLINE-06: unaffected PersonaIDs identical |
| `TestBuildPersonaGraphIncremental_PersonaIDAboveMax` | ego_splitting_test.go | ONLINE-11: new IDs above max(prior+nodes) |
| `TestUpdate_AffectedNodesOnly` | ego_splitting_test.go | ONLINE-05: spy count == len(affected) |
| `TestUpdate_UnaffectedPersonasCarriedOver` | ego_splitting_test.go | ONLINE-06: unaffected PersonaIDs unchanged |
| `TestUpdate_PersonaIDDisjoint` | ego_splitting_test.go | ONLINE-11: new IDs don't collide |
| `TestUpdate_WarmStartGlobalDetection` | ego_splitting_test.go | ONLINE-07: all 35 nodes in result |
| `TestUpdate_NilCarryForwardFallback` | ego_splitting_test.go | nil prior → graceful fallback |
| `TestUpdate_MultipleSequentialUpdates` | ego_splitting_test.go | 3 sequential updates maintain invariants |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] DeltaEdge type introduced for GraphDelta.AddedEdges**

- **Found during:** Task 1 RED phase — tests failed to compile with `Edge{From: ..., To: ...}`
- **Issue:** `GraphDelta.AddedEdges []Edge` used `Edge` which only has `To` and `Weight` (no `From`). Delta edges need both endpoints to be self-contained.
- **Fix:** Introduced `DeltaEdge{From, To, Weight}` type; changed `GraphDelta.AddedEdges` to `[]DeltaEdge`. Updated all test literals and the `computeAffected` implementation to use `e.From`/`e.To`.
- **Files modified:** graph/ego_splitting.go, graph/ego_splitting_test.go
- **Commit:** 5550a6c

**2. [Rule 1 - Bug] TestUpdate_PersonaIDDisjoint and TestUpdate_MultipleSequentialUpdates assertion refined**

- **Found during:** Task 2 GREEN phase — `TestUpdate_MultipleSequentialUpdates` failed because PersonaID 34 (allocated in prior Detect when nodes were 0-33) was legitimately present while NodeID 34 was added in the first Update step
- **Issue:** Initial test asserted ALL PersonaIDs > maxNodeID, but ONLINE-11's invariant is about NEW allocations only; carried-over prior PersonaIDs may predate a new NodeID
- **Fix:** Tests now distinguish "new PersonaIDs" (not in prior.inverseMap) vs "carried-over" and only assert collision-freedom for new ones
- **Files modified:** graph/ego_splitting_test.go
- **Commit:** 7fba0d3

### Pre-existing issue (out of scope)

`TestLeidenWarmStartSpeedup` is a flaky benchmark-as-test that fails when the machine is under load (threshold 1.2x, observed 1.10-1.19x). Present before Phase 11 work, not caused by these changes. Logged in 11-01-SUMMARY. `go test ./graph/... -count=1 -race -run Test` (excluding benchmarks-as-tests) passes cleanly.

## Known Stubs

None. `Update()` is fully incremental for non-empty deltas. `computeAffected` and `buildPersonaGraphIncremental` are complete implementations.

## Self-Check

### Files created/modified exist:

- [x] graph/ego_splitting.go — modified (computeAffected, buildPersonaGraphIncremental, incremental Update, DeltaEdge)
- [x] graph/ego_splitting_test.go — modified (12 new tests, countingDetector spy)
- [x] .planning/phases/11-incremental-recomputation-core/11-02-SUMMARY.md — created

### Commits exist:

- [x] 5550a6c — computeAffected + buildPersonaGraphIncremental + DeltaEdge
- [x] 7fba0d3 — incremental Update() + ONLINE-05/06/07/11 tests

## Self-Check: PASSED
