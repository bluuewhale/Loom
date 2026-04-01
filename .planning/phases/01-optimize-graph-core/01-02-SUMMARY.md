---
phase: 01-optimize-graph-core
plan: 02
subsystem: graph
tags: [performance, optimization, bfs, sync-pool, louvain, leiden]
dependency_graph:
  requires: [01-01]
  provides: [bfs-cursor-fix, buildSupergraph-pre-sized, subgraph-pool]
  affects: [graph/leiden.go, graph/louvain.go, graph/graph.go]
tech_stack:
  added: [sync.Pool]
  patterns: [cursor-based BFS, pool-based allocation, deterministic sorted writes]
key_files:
  created: []
  modified:
    - graph/leiden.go
    - graph/louvain.go
    - graph/graph.go
decisions:
  - "BFS cursor (head int) replaces queue[1:] slice in refinePartition; queue backing array reused across communities"
  - "buildSupergraph n<e.To single-pass dedup reverted — changed adjacency insertion order, failing accuracy tests; pre-sized maps (EdgeCount() / len(commList)) retained as the allocation gain"
  - "buildSupergraph write phase now sorts selfLoopNodes and interKeys before AddEdge — makes adjacency layout deterministic (bonus over old random map-iteration write order)"
  - "sync.Pool for Subgraph seen-map — eliminates per-call map allocation across ~10K EgoSplitting ego-net builds"
metrics:
  duration: "13min"
  completed_date: "2026-04-01"
  tasks_completed: 2
  files_modified: 3
---

# Phase 01 Plan 02: BFS cursor + buildSupergraph pre-sized maps + Subgraph pool Summary

BFS cursor fix eliminates queue[1:] backing-array abandonment in Leiden refinePartition; buildSupergraph gets pre-sized maps and deterministic sorted write order; Subgraph seen-map pooled via sync.Pool to eliminate per-call allocation in EgoSplitting's 10K ego-net builds.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | BFS cursor fix + buildSupergraph pre-sized maps | 296899e | graph/leiden.go, graph/louvain.go |
| 2 | Subgraph seen-map pooling via sync.Pool | b227a24 | graph/graph.go |

## What Was Built

### Task 1: BFS cursor fix (leiden.go)

Replaced `queue = queue[1:]` with a `head int` cursor in `refinePartition`:
- `var queue []NodeID` declared before outer community loop — backing array reused across communities
- Inside each BFS: `queue = queue[:0]; queue = append(queue, start); head := 0`
- Loop condition: `for head < len(queue)` with `head++` instead of slice-shrinking

Prevents per-BFS backing-array abandonment when processing many communities.

### Task 1: buildSupergraph pre-sized maps + sorted writes (louvain.go)

- `interEdges` pre-sized to `g.EdgeCount()` (upper bound for inter-community entries)
- `selfLoops` pre-sized to `len(commList)` (one entry per community)
- Write phase now sorts `selfLoopNodes` and `interKeys` before calling `AddEdge` — makes supergraph adjacency layout fully deterministic regardless of Go map iteration order

**Deviation:** The plan specified a `n < e.To` single-pass dedup to eliminate `/2.0` divisions. This was implemented and caused accuracy test regressions: the `n < e.To` guard changed the map insertion sequence for `interEdges`, which changed adjacency list order in the resulting supergraph (Go map iteration is hash-order-dependent), which shifted Louvain's local optima under fixed seeds. Root cause confirmed by direct supergraph comparison (weights identical, adjacency order differed in 34/103 Polbooks ego-nets). The fix was to retain the original double-accumulation + `/2.0` pattern while adding the pre-sized maps. The deterministic sorted write (sorting keys before AddEdge) was added as a bonus improvement, also confirming tests pass reliably.

### Task 2: Subgraph seen-map pool (graph.go)

Added package-level `sync.Pool`:
```go
var subgraphSeenPool = sync.Pool{
    New: func() any {
        m := make(map[[2]NodeID]struct{}, 32)
        return &m
    },
}
```

`Subgraph()` now:
1. Gets `*map[[2]NodeID]struct{}` from pool
2. Clears it with `for k := range seen { delete(seen, k) }`
3. Uses it as before
4. Returns it via `defer subgraphSeenPool.Put(seenPtr)`

Eliminates one `make(map)` per `Subgraph()` call — reduces allocation pressure for the ~10K calls during EgoSplitting `buildPersonaGraph`. Pool is goroutine-safe; `runParallelEgoNets` workers each get their own map instance.

## Verification Results

```
go test ./graph/... -count=1 -timeout=120s
ok  github.com/bluuewhale/loom/graph  14.407s
```

All existing tests pass including accuracy tests (NMI, OmegaIndex), warm-start tests, race tests.

### Benchmark Results (no regression)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| BenchmarkLouvain10K | ~82ms | 29.5MB | 75,710 |
| BenchmarkLeiden10K | ~88ms | 32.4MB | 93,929 |
| BenchmarkEgoSplitting10K | ~212ms | 145.6MB | 798,900 |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] n<e.To single-pass dedup caused accuracy test regressions**

- **Found during:** Task 1 implementation
- **Issue:** Plan specified `n <= e.To` (self-loops) and `n < e.To` (inter-community) guards to process each undirected edge once, eliminating `/2.0` divisions. This was implemented correctly in terms of weights (supergraph TotalWeight was identical, direct edge-for-edge comparison confirmed identical edge sets) but changed the map insertion order for `interEdges`. Go's hash map iteration is non-deterministic per insertion sequence → different `AddEdge` call order → different adjacency list layout in supergraph → different Louvain outcomes under fixed seeds. 34/103 Polbooks ego-nets had adjacency order differences. KarateClub Omega dropped from 0.36 to 0.30 (right at threshold), Polbooks Omega dropped to 0.21 (below 0.30 threshold).
- **Fix:** Reverted to original double-accumulation + canonical key + `/2.0` approach which preserves identical insertion order and adjacency layout. Added `slices.Sort` on keys before write phase to make layout deterministic going forward. Pre-sized maps retained.
- **Files modified:** graph/louvain.go
- **Commit:** 296899e

## Known Stubs

None — all changes are functional optimizations with no placeholder logic.

## Self-Check

### Created files exist:
- `.planning/phases/01-optimize-graph-core/01-02-SUMMARY.md` — this file ✓

### Commits exist:
- 296899e — feat(01-02): BFS cursor fix in refinePartition + buildSupergraph pre-sized maps ✓
- b227a24 — feat(01-02): pool Subgraph seen-map via sync.Pool ✓

## Self-Check: PASSED
