# Phase 13: Correctness Hardening and Race Safety - Research

**Researched:** 2026-03-31
**Domain:** Go testing — structural invariant verification, race-detector coverage
**Confidence:** HIGH

---

## Summary

Phase 13 closes the final two open requirements for v1.3: ONLINE-12 (structural
invariant guarantees on `Update()` results) and ONLINE-13 (race-safety proof for
concurrent `Update()` calls on distinct detector instances). Both requirements are
purely about tests — no production code changes are needed or expected.

The existing test suite covers API contracts, carry-forward fields, and incremental
behaviour extensively (Phases 10–12). What it does NOT cover is: (1) verifying the
bidirectional consistency invariant between `Communities` and `NodeCommunities` in
the result returned by `Update()`, and (2) explicitly running concurrent `Update()`
calls (as opposed to the existing `TestEgoSplittingConcurrentDetect` which only
covers `Detect()`).

For ONLINE-13, the key finding is that the production code is already race-safe by
construction: each `Update()` call operates exclusively on its own local state
(detector instance, `prior` value copy, `g` pointer). The two package-level `sync.Pool`
variables (`louvainStatePool`, `leidenStatePool`) are safe because `sync.Pool` itself
is concurrency-safe. The goroutine worker pool in Phase 12 operates within a single
`Update()` call and has no cross-call shared state. The race test therefore just
needs to instantiate concurrent `Update()` calls on distinct instances and let
`go test -race` confirm the absence of data races — no production fixes expected.

**Primary recommendation:** Add one table-driven invariant-checking test for
ONLINE-12 and one concurrent-`Update()` stress test for ONLINE-13. Both tests
should be self-contained in `graph/ego_splitting_test.go`; no new files required.

---

## Project Constraints (from CLAUDE.md)

- Do NOT add `Co-Authored-By: Claude` or any Claude Code co-author trailer to
  commit messages.
- After completing any code, design, or plan, a subagent review is mandatory before
  considering the task done.
- External dependencies: stdlib only. No new imports permitted (stated in
  REQUIREMENTS.md Out of Scope).
- Directed graph incremental support is out of scope.
- `Detect()` behaviour must not be modified.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ONLINE-12 | `Update()` result satisfies all existing result invariants: every original node (including newly added ones) appears in at least one community; `NodeCommunities` and `Communities` are mutually consistent | Invariants are fully defined by the `Detect()` code path and existing test patterns (see §Invariant Taxonomy below). No production code change needed — only tests. |
| ONLINE-13 | `Update()` is concurrent-safe — `go test -race` passes on concurrent `Update()` calls on distinct detector instances | Production code is already race-safe by construction (see §Race Safety Analysis). Test mirrors `TestEgoSplittingConcurrentDetect` but calls `Update()` instead of `Detect()`. |
</phase_requirements>

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` (stdlib) | Go 1.21+ | Test framework | Project stdlib-only constraint |
| `sync` (stdlib) | Go 1.21+ | WaitGroup for concurrent test | Already used in `TestEgoSplittingConcurrentDetect` |

No new imports. No new files required beyond what is already in `graph/`.

**Version verification:** Project uses standard library only. No external packages.

---

## Invariant Taxonomy (ONLINE-12)

These are the concrete invariants that `Update()` results MUST satisfy — derived
from reading `Detect()` and the existing test coverage.

### Invariant 1: All-Nodes Present in NodeCommunities

Every node currently in the graph `g` must appear as a key in
`result.NodeCommunities` with at least one community index.

**Concrete check:**
```go
for _, id := range g.Nodes() {
    comms, ok := result.NodeCommunities[id]
    if !ok || len(comms) == 0 {
        t.Errorf("node %d missing or has no community", id)
    }
}
```

**Coverage gap:** Existing tests check this for specific scenarios
(`TestUpdate_WarmStartGlobalDetection` checks all 35 nodes after one edge+node
addition). But there is no systematic table-driven test across delta types:
empty-delta, isolated-node-only delta, edge-between-existing-nodes delta,
multi-node batch delta.

### Invariant 2: NodeCommunities → Communities Consistency (forward)

For every entry `(nodeID, commIndices)` in `NodeCommunities`, each community
index `i` must be a valid index into `Communities` (i.e. `0 <= i <
len(Communities)`) and `nodeID` must appear in `Communities[i]`.

**Concrete check:**
```go
for nodeID, commIndices := range result.NodeCommunities {
    for _, ci := range commIndices {
        if ci < 0 || ci >= len(result.Communities) {
            t.Errorf("NodeCommunities[%d] has out-of-range index %d", nodeID, ci)
            continue
        }
        found := false
        for _, member := range result.Communities[ci] {
            if member == nodeID { found = true; break }
        }
        if !found {
            t.Errorf("NodeCommunities[%d] claims community %d but node absent from Communities[%d]", nodeID, ci, ci)
        }
    }
}
```

### Invariant 3: Communities → NodeCommunities Consistency (reverse)

For every community `i` and every member `nodeID` in `Communities[i]`,
`NodeCommunities[nodeID]` must contain `i`.

**Concrete check:**
```go
for ci, members := range result.Communities {
    for _, nodeID := range members {
        comms, ok := result.NodeCommunities[nodeID]
        if !ok {
            t.Errorf("Communities[%d] member %d absent from NodeCommunities", ci, nodeID)
            continue
        }
        found := false
        for _, c := range comms { if c == ci { found = true; break } }
        if !found {
            t.Errorf("Communities[%d] member %d does not list %d in NodeCommunities", ci, nodeID, ci)
        }
    }
}
```

### Invariant 4: No Empty Communities

Every `Communities[i]` slice must be non-empty. The compaction step in both
`Detect()` and `Update()` filters empty slots — verify it works in `Update()`.

**Concrete check:**
```go
for ci, members := range result.Communities {
    if len(members) == 0 {
        t.Errorf("Communities[%d] is empty after compaction", ci)
    }
}
```

### Invariant 5: No Duplicate Community Indices Per Node

`NodeCommunities[nodeID]` must contain no duplicate community indices. The
deduplication step in both `Detect()` and `Update()` handles this — verify it
holds after `Update()`.

**Concrete check:**
```go
for nodeID, comms := range result.NodeCommunities {
    seen := make(map[int]struct{})
    for _, ci := range comms {
        if _, dup := seen[ci]; dup {
            t.Errorf("NodeCommunities[%d] has duplicate community index %d", nodeID, ci)
        }
        seen[ci] = struct{}{}
    }
}
```

### Invariant 6: Communities Contiguous (0-indexed, no gaps)

After compaction, community indices in `NodeCommunities` must be contiguous
`[0, len(Communities))` with no gaps.

**Concrete check:**
```go
usedIndices := make(map[int]struct{})
for _, comms := range result.NodeCommunities {
    for _, ci := range comms { usedIndices[ci] = struct{}{} }
}
for i := 0; i < len(result.Communities); i++ {
    if _, ok := usedIndices[i]; !ok {
        t.Errorf("community index %d in range [0,%d) is unused", i, len(result.Communities))
    }
}
```

---

## Test Case Matrix (ONLINE-12)

The table-driven test should exercise `Update()` across these delta types and
verify all six invariants above for each:

| Test Case | Delta | Graph State | Why It Matters |
|-----------|-------|-------------|----------------|
| empty-delta | `{}` | KarateClub | Invariants hold when prior is returned unchanged |
| isolated node | `{AddedNodes: [34]}` | KarateClub + node 34 | Isolated fast-path in `buildPersonaGraphIncremental` |
| edge between existing nodes | `{AddedEdges: [{16,24,1}]}` | KarateClub | Full ego-net rebuild for 7 affected nodes |
| node + connecting edge | `{AddedNodes:[34], AddedEdges:[{0,34,1}]}` | KarateClub + node 34 connected | Non-isolated new node |
| batch: 3 nodes | `{AddedNodes: [34,35,36]}` | KarateClub + 3 isolated | Multiple isolated nodes in one delta |
| nil carry-forward fallback | `{AddedNodes:[34]}` with zero-value `prior` | KarateClub + node 34 | Falls back to `Detect()` — invariants must still hold |

**Recommended helper:** Extract the 6 invariant checks into a helper function
`assertResultInvariants(t *testing.T, g *Graph, result OverlappingCommunityResult)`
that can be called from any existing or future test. This avoids duplication and
makes the checks reusable if new delta scenarios are added in v1.4.

---

## Race Safety Analysis (ONLINE-13)

### Shared state audit

| State | Location | Shared across `Update()` calls? | Safe? |
|-------|----------|---------------------------------|-------|
| `louvainStatePool` | `louvain_state.go:21` | Yes — package-level `sync.Pool` | YES — `sync.Pool` is goroutine-safe by design |
| `leidenStatePool` | `leiden_state.go:23` | Yes — package-level `sync.Pool` | YES — same |
| `ErrEmptyGraph` | `ego_splitting.go:10` | Yes — immutable sentinel | YES — read-only |
| `ErrDirectedNotSupported` | `detector.go:7` | Yes — immutable sentinel | YES — read-only |
| `egoSplittingDetector.opts` | per-instance struct | No — each goroutine creates its own detector OR reuses same instance | SAFE if distinct instances (per ONLINE-13 constraint) |
| `OverlappingCommunityResult` | value passed as `prior` | No — Go value semantics; outer maps are copied in `buildPersonaGraphIncremental` | YES |
| `*Graph g` | pointer passed by caller | Caller must not mutate `g` concurrently — same constraint as `Detect()` | SAFE if callers use distinct `*Graph` instances |
| `runParallelEgoNets` goroutine pool | per-`Update()` call | No — pool is created and destroyed within one call | YES |
| `cloneDetector` per-worker clones | per-`Update()` call | No — each worker gets its own fresh clone | YES |

**Conclusion:** There is no shared mutable state across distinct `Update()` calls.
The race test is expected to pass without any production code changes. Its sole
purpose is to provide a regression guard detectable by `go test -race`.

### Known flaky tests to skip

`TestLouvainWarmStartSpeedup` and `TestLeidenWarmStartSpeedup` are pre-existing
flaky tests under `-race` (documented in Phase 12 SUMMARY). The concurrent Update
test must NOT be designed in a way that could be affected by timing, so it should
avoid any speedup assertions and use functional correctness checks only.

### Test design for ONLINE-13

Mirror `TestEgoSplittingConcurrentDetect` (line 459 in `ego_splitting_test.go`).

```go
// TestEgoSplittingConcurrentUpdate validates that concurrent Update() calls on
// distinct detector instances produce no data races. Run with -race. (ONLINE-13)
func TestEgoSplittingConcurrentUpdate(t *testing.T) {
    const goroutines = 4
    const iterations = 3

    var wg sync.WaitGroup
    for i := 0; i < goroutines; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            g := buildGraph(testdata.KarateClubEdges)
            det := NewOnlineEgoSplitting(EgoSplittingOptions{
                LocalDetector:  NewLouvain(LouvainOptions{Seed: int64(idx + 1)}),
                GlobalDetector: NewLouvain(LouvainOptions{Seed: int64(idx + 1)}),
            })
            prior, err := det.Detect(g)
            if err != nil {
                t.Errorf("goroutine %d Detect: %v", idx, err)
                return
            }
            for j := 0; j < iterations; j++ {
                newNode := NodeID(34 + j)
                g.AddNode(newNode, 1.0)
                g.AddEdge(0, newNode, 1.0)
                delta := GraphDelta{
                    AddedNodes: []NodeID{newNode},
                    AddedEdges: []DeltaEdge{{From: 0, To: newNode, Weight: 1.0}},
                }
                prior, err = det.Update(g, delta, prior)
                if err != nil {
                    t.Errorf("goroutine %d iteration %d Update: %v", idx, j, err)
                    return
                }
                if len(prior.Communities) == 0 {
                    t.Errorf("goroutine %d iteration %d: no communities", idx, j)
                    return
                }
            }
        }(i)
    }
    wg.Wait()
}
```

Each goroutine: distinct `*Graph`, distinct `*egoSplittingDetector`, independent
`prior` — no shared mutable state. The `-race` flag catches any accidental sharing.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead |
|---------|-------------|-------------|
| Race detector | Custom synchronization checker | `go test -race` (built into Go toolchain) |
| Invariant assertions | Complex reflection-based validators | Inline table-driven assertions using stdlib `testing.T` |
| Concurrent test harness | Custom goroutine lifecycle manager | `sync.WaitGroup` (already used in `TestEgoSplittingConcurrentDetect`) |

---

## Architecture Patterns

### Recommended structure

No new files needed. All tests go into `graph/ego_splitting_test.go`, following the
existing pattern:
- Helper function `assertResultInvariants(t, g, result)` — reusable checker
- `TestUpdateResultInvariants` — table-driven test for ONLINE-12
- `TestEgoSplittingConcurrentUpdate` — concurrent stress test for ONLINE-13

### Existing helper inventory

| Helper | Location | Reusable for Phase 13? |
|--------|----------|------------------------|
| `buildGraph(edges)` | `testhelpers_test.go:63` | YES — use for all delta scenarios |
| `makeTriangle()` | `ego_splitting_test.go:118` | YES — lightweight smoke cases |
| `makeBarbell()` | `ego_splitting_test.go:126` | YES — optional edge case |
| `makeStar(n)` | `ego_splitting_test.go:110` | OPTIONAL |
| `countingDetector` | `ego_splitting_test.go:1167` | NO — not needed for invariant/race tests |
| `partitionToGroundTruth` | (used in accuracy tests) | NO — not needed here |
| `nmi` | `testhelpers_test.go:13` | NO |

### Anti-patterns to avoid

- **Timing assertions in the concurrent test:** The race test only needs to verify
  correctness and let `-race` flag races. Any `time.Since` or speedup check will be
  flaky under `-race` (3x overhead). Do not add timing to `TestEgoSplittingConcurrentUpdate`.
- **Sharing a single `*Graph` across goroutines in the race test:** `Graph` is not
  goroutine-safe for concurrent writes. Each goroutine must own its own `*Graph`.
- **Sharing a single detector across goroutines:** The race test constraint
  (ONLINE-13) is "distinct detector instances" — one detector per goroutine.
- **Testing `prior` mutation:** `OverlappingCommunityResult` is passed by value
  to `Update()`; its outer maps are copied in `buildPersonaGraphIncremental`. The
  inner maps are shared read-only (shallow copy). Tests must not attempt to write
  to `prior` after passing it to `Update()`.

---

## Common Pitfalls

### Pitfall 1: Invariant check misses the isolated fast-path
**What goes wrong:** The isolated-node fast-path in `buildPersonaGraphIncremental`
returns early without going through the full compaction pipeline. If the invariant
test only exercises edge-addition deltas, it skips this path.
**Why it happens:** The fast-path is a separate return branch (line 721 of
`ego_splitting.go`). It reassigns `globalPartition` directly from the
`isolatedOnly` branch.
**How to avoid:** Include `{AddedNodes: [34]}` (no edges) as a test case.
The test matrix above includes this.

### Pitfall 2: Parallel `g.AddNode/AddEdge` race in concurrent test
**What goes wrong:** If the test mutates `g` in one goroutine while another
goroutine calls `g.Neighbors()` inside `Update()`, the race detector will
flag it — but this is a test design bug, not a production bug.
**How to avoid:** Each goroutine uses its OWN `g := buildGraph(...)`. Never
share the graph across goroutines in the race test.

### Pitfall 3: Off-by-one in community index range check
**What goes wrong:** After compaction, community indices are `[0, len(Communities))`.
A node could have `NodeCommunities[id] = [len(Communities)]` if the remap step
has a bug. The invariant check must use strict `<` not `<=`.
**How to avoid:** The forward consistency check uses `ci >= len(result.Communities)`.

### Pitfall 4: Invariant holds for `Detect()` but not `Update()` — empty community
**What goes wrong:** The `Update()` path builds `communities := make([][]NodeID,
maxComm+1)` then filters empty slots. If `maxComm` is computed incorrectly
(e.g., off by one due to the `isolatedOnly` singleton assignment), a
`Communities[i]` slot could remain empty.
**How to avoid:** The invariant 4 check (`len(members) == 0`) will catch this.
Include the nil-carry-forward fallback (falls back to `Detect()`) as one test
case to confirm invariants hold even after a cold-start fallback.

### Pitfall 5: `sync.Pool` across goroutines — false race concern
**What goes wrong:** A reviewer might worry that `louvainStatePool` and
`leidenStatePool` are package-level `sync.Pool` variables shared across goroutines,
constituting a data race.
**Why it isn't:** `sync.Pool.Get()` and `Put()` are explicitly documented as safe
for concurrent use. The race detector knows about `sync.Pool` internals and does
not flag it. This is HIGH confidence — verified by the fact that
`TestEgoSplittingConcurrentDetect` already exercises the same pool concurrently
and passes `-race`.

---

## Code Examples

### assertResultInvariants helper
```go
// Source: derived from existing invariants in ego_splitting.go Detect() and Update()
func assertResultInvariants(t *testing.T, g *Graph, result OverlappingCommunityResult) {
    t.Helper()

    // Invariant 1: all nodes present in NodeCommunities with at least one community.
    for _, id := range g.Nodes() {
        comms, ok := result.NodeCommunities[id]
        if !ok || len(comms) == 0 {
            t.Errorf("node %d: missing from NodeCommunities or has no community", id)
        }
    }

    // Invariant 4: no empty communities.
    for ci, members := range result.Communities {
        if len(members) == 0 {
            t.Errorf("Communities[%d] is empty", ci)
        }
    }

    // Invariant 2: NodeCommunities -> Communities forward consistency.
    for nodeID, commIndices := range result.NodeCommunities {
        // Invariant 5: no duplicates.
        seen := make(map[int]struct{})
        for _, ci := range commIndices {
            if _, dup := seen[ci]; dup {
                t.Errorf("NodeCommunities[%d]: duplicate index %d", nodeID, ci)
            }
            seen[ci] = struct{}{}

            if ci < 0 || ci >= len(result.Communities) {
                t.Errorf("NodeCommunities[%d]: index %d out of range [0,%d)", nodeID, ci, len(result.Communities))
                continue
            }
            found := false
            for _, member := range result.Communities[ci] {
                if member == nodeID { found = true; break }
            }
            if !found {
                t.Errorf("NodeCommunities[%d] claims community %d but node absent from Communities[%d]", nodeID, ci, ci)
            }
        }
    }

    // Invariant 3: Communities -> NodeCommunities reverse consistency.
    for ci, members := range result.Communities {
        for _, nodeID := range members {
            comms, ok := result.NodeCommunities[nodeID]
            if !ok {
                t.Errorf("Communities[%d] member %d absent from NodeCommunities", ci, nodeID)
                continue
            }
            found := false
            for _, c := range comms { if c == ci { found = true; break } }
            if !found {
                t.Errorf("Communities[%d] member %d: index %d not in NodeCommunities", ci, nodeID, ci)
            }
        }
    }

    // Invariant 6: contiguous indices.
    usedIndices := make(map[int]struct{})
    for _, comms := range result.NodeCommunities {
        for _, ci := range comms { usedIndices[ci] = struct{}{} }
    }
    for i := 0; i < len(result.Communities); i++ {
        if _, ok := usedIndices[i]; !ok {
            t.Errorf("community index %d in [0,%d) unused", i, len(result.Communities))
        }
    }
}
```

### Table-driven invariant test skeleton
```go
// TestUpdateResultInvariants verifies ONLINE-12 across all delta types.
func TestUpdateResultInvariants(t *testing.T) {
    det := NewOnlineEgoSplitting(EgoSplittingOptions{
        LocalDetector:  NewLouvain(LouvainOptions{Seed: 42}),
        GlobalDetector: NewLouvain(LouvainOptions{Seed: 42}),
    })
    base := buildGraph(testdata.KarateClubEdges) // 34 nodes, 0-33
    prior, err := det.Detect(base)
    if err != nil { t.Fatalf("Detect: %v", err) }

    tests := []struct {
        name  string
        setup func() (*Graph, GraphDelta, OverlappingCommunityResult)
    }{
        {
            name: "empty-delta",
            setup: func() (*Graph, GraphDelta, OverlappingCommunityResult) {
                g := buildGraph(testdata.KarateClubEdges)
                return g, GraphDelta{}, prior
            },
        },
        {
            name: "isolated-node",
            setup: func() (*Graph, GraphDelta, OverlappingCommunityResult) {
                g := buildGraph(testdata.KarateClubEdges)
                g.AddNode(34, 1.0)
                return g, GraphDelta{AddedNodes: []NodeID{34}}, prior
            },
        },
        // ... (edge-between-existing, node+edge, batch, nil-prior-fallback)
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            g, delta, p := tt.setup()
            result, err := det.Update(g, delta, p)
            if err != nil { t.Fatalf("Update: %v", err) }
            assertResultInvariants(t, g, result)
        })
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Serial ego-net detection | Parallel goroutine pool (GOMAXPROCS workers) | Phase 12 | Worker clones are per-call; no cross-call sharing |
| No explicit race test for Update() | `TestEgoSplittingConcurrentDetect` covers Detect only | Phase 10 | Phase 13 adds Update() coverage |
| Invariant checked ad-hoc in individual tests | Reusable `assertResultInvariants` helper | Phase 13 (new) | Enables systematic table-driven coverage |

---

## Open Questions

1. **Should the invariant test also call `Detect()` for comparison?**
   - What we know: ONLINE-12 says "all existing result invariants" — these are defined
     by both `Detect()` and `Update()` producing `OverlappingCommunityResult`.
   - What's unclear: Whether the requirement implies `Update()` must produce
     structurally identical output to `Detect()` on the same graph, or just that it
     satisfies the same invariants independently.
   - Recommendation: Invariants independently (not identity). Requiring identical
     output is too strict — `Update()` uses a different warm-start path and may
     produce different community numbering.

2. **How many goroutines and iterations in the concurrent test?**
   - What we know: `TestEgoSplittingConcurrentDetect` uses 4 goroutines × 5 iterations.
   - Recommendation: Use 4 goroutines × 3 iterations for `Update()` (3 sequential
     node additions per goroutine). This exercises both the isolated fast-path and
     the warm-start path without making the test slow under `-race`.

---

## Environment Availability

Step 2.6: SKIPPED — phase is purely code changes (new test functions in existing
`graph/ego_splitting_test.go`). No external dependencies beyond the Go toolchain,
which is confirmed present (`go test ./graph/... -race` currently passes in 1.8s).

---

## Validation Architecture

Step 2.6 note: `workflow.nyquist_validation` is explicitly `false` in
`.planning/config.json`. Validation Architecture section is OMITTED per
execution-flow rules.

---

## Sources

### Primary (HIGH confidence)
- Direct code reading: `graph/ego_splitting.go` — full `Detect()` and `Update()`
  pipelines, invariant-enforcing compaction and dedup steps
- Direct code reading: `graph/ego_splitting_test.go` — existing test coverage
  inventory, `countingDetector` spy, `TestEgoSplittingConcurrentDetect` pattern
- Direct code reading: `graph/louvain_state.go` — `sync.Pool` pattern; confirmed
  goroutine-safe by design
- Direct code reading: `graph/testhelpers_test.go` — available helpers
  (`buildGraph`, `perturbGraph`, etc.)
- `go test ./graph/... -count=1 -race` run result: PASS in 1.8s — confirms no
  pre-existing races in current codebase

### Secondary (MEDIUM confidence)
- Phase 12 SUMMARY.md — confirmed `countingDetector` mutex fix, worker clone
  pattern, and that `go test -race` passes post-Phase-12

### Tertiary (LOW confidence)
- None

---

## Metadata

**Confidence breakdown:**
- Invariant taxonomy: HIGH — derived directly from production `Detect()`/`Update()` code
- Race safety analysis: HIGH — package-level vars audited directly; `-race` run confirmed clean
- Test design patterns: HIGH — mirroring existing `TestEgoSplittingConcurrentDetect`
- No production code changes needed: HIGH — all reasoning derived from direct code reading

**Research date:** 2026-03-31
**Valid until:** 2026-05-31 (stable domain; no external dependencies)
