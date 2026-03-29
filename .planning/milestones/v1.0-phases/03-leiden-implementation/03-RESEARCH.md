# Phase 03: Leiden Implementation - Research

**Researched:** 2026-03-29
**Domain:** Go community detection — Leiden algorithm with BFS refinement phase
**Confidence:** HIGH

## Summary

Phase 03 implements a full `LeidenDetector` in pure Go inside `package graph`. The key distinguishing feature of Leiden vs. Louvain is the **refinement phase**: after each local-move pass (Phase 1), every community is checked for internal connectivity via BFS; any disconnected community is split into its connected components before supergraph aggregation (Phase 3). The aggregation then uses the refined partition, not the raw local-move partition.

All algorithmic helpers from the Louvain implementation — `phase1`, `deltaQ`, `buildSupergraph`, `normalizePartition`, `reconstructPartition` — are directly reusable without modification. The implementation adds two new files: `leiden.go` (main `Detect` method) and `leiden_state.go` (`leidenState` struct + constructor), and a new test file `leiden_test.go` that mirrors `louvain_test.go` and adds an NMI accuracy assertion.

**Primary recommendation:** Implement `leidenDetector.Detect` as a near-copy of `louvainDetector.Detect`, inserting a BFS refinement step between Phase 1 and supergraph aggregation. The only structural difference is that `buildSupergraph` is called with `state.refinedPartition` instead of `state.partition`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **Connectivity detection:** BFS within each community — simple, readable, fits small-to-medium community sizes
- **Disconnected sub-communities:** split each connected component into its own community (components become separate communities after refinement)
- **Refinement timing:** run every iteration (after every local-move pass) — this is the true Leiden behavior per Traag et al. (2019)
- **Aggregation partition:** use the refined partition (not local-move partition) for supergraph construction — this is the key correctness requirement that distinguishes Leiden from Louvain
- **Code reuse:** Reuse Louvain helpers directly: `phase1`, `deltaQ`, `buildSupergraph`, `normalizePartition`, `reconstructPartition` — same package, no visibility barriers, no duplication
- **New `leidenState` struct** in `leiden_state.go` with its own fields including `refinedPartition map[NodeID]int` alongside the standard partition/commStr/rng fields
- **Files:** `leiden.go` + `leiden_state.go` (mirrors the Louvain file split)
- **NMI implementation:** unexported test helper `nmi(p1, p2 map[NodeID]int) float64` inside `leiden_test.go` — keeps it test-only, no public API surface added
- **Test file:** separate `leiden_test.go` (mirrors `louvain_test.go` structure)
- **Ground-truth fixture:** reuse existing `KarateClub()` and `KarateGroundTruth()` from `graph/testdata/karate.go`

### Claude's Discretion

- Internal `leidenState` field names and BFS helper implementation details
- Whether to use a visited map or slice for BFS queue
- Self-loop and isolated node handling in BFS (skip self-loops; isolated nodes stay in singleton communities)
- How `refinedPartition` is initialized (copy of local-move partition, then split)

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| LEID-01 | Phase 1 local move — same ΔQ optimization as Louvain Phase 1 | `phase1` function exists and is fully reusable; no changes needed |
| LEID-02 | Refinement phase — ensure internal connectivity per community each iteration; BFS split of disconnected communities | BFS connected-component extraction pattern documented below; runs after every `phase1` call |
| LEID-03 | Phase 3 aggregation — build supergraph using the refined partition | `buildSupergraph(currentGraph, state.refinedPartition)` replaces `buildSupergraph(currentGraph, state.partition)`; no changes to `buildSupergraph` itself |
| LEID-04 | Karate Club NMI ≥ 0.7 vs ground-truth 2-community partition | `KarateClubPartition` fixture has 34-node map; NMI formula is entropy-based (documented below); Louvain achieves Q > 0.35 on the same graph, Leiden should match or exceed |
</phase_requirements>

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Standard library (`math`, `math/rand`) | Go stdlib | NMI entropy computation, RNG | Already used in `louvain_state.go`; no new imports required for core algorithm |
| Standard library (`math`) | Go stdlib | `math.Log2` for NMI calculation in test | Required for entropy in `leiden_test.go` |

No external dependencies needed. The implementation is pure Go using the existing `package graph` types.

### Reusable Internal Functions

| Function | Signature | Reuse Pattern |
|----------|-----------|---------------|
| `phase1` | `phase1(g *Graph, state *louvainState, resolution, m float64) int` | Called with `leidenState.louvainState` or adapted state — see note below |
| `deltaQ` | `deltaQ(g, n, comm, partition, commStr, resolution, m)` | Unchanged |
| `buildSupergraph` | `buildSupergraph(g *Graph, partition map[NodeID]int)` | Called with `refinedPartition` instead of raw partition |
| `normalizePartition` | `normalizePartition(partition map[NodeID]int) map[NodeID]int` | Unchanged |
| `reconstructPartition` | `reconstructPartition(origNodes, nodeMapping, superPartition)` | Unchanged |

**Note on `phase1` reuse:** `phase1` takes a `*louvainState`. Two options:
1. Embed `*louvainState` inside `leidenState` and pass `state.louvainState` to `phase1`.
2. Give `leidenState` the same `partition`/`commStr`/`rng` fields and pass it as `*louvainState` (requires type alias or field extraction).

The simplest approach is to have `leidenState` embed `*louvainState` (or hold a `*louvainState` directly) and call `phase1(g, state.ls, resolution, m)`. This avoids duplicating the `phase1` signature.

**Alternatively** (also valid): Give `leidenState` identical fields to `louvainState` (`partition`, `commStr`, `rng`) plus `refinedPartition`, and extract a `*louvainState`-shaped view when calling `phase1`. Since both structs are in the same package, this is straightforward.

## Architecture Patterns

### Recommended File Structure

```
graph/
├── leiden.go            # leidenDetector.Detect() method + BFS refinement helper
├── leiden_state.go      # leidenState struct + newLeidenState() constructor
├── leiden_test.go       # TestLeidenKarateClub, TestLeidenConnectedCommunities, nmi() helper
```

### Pattern 1: leidenState Structure

```go
// leiden_state.go
type leidenState struct {
    partition        map[NodeID]int  // from local-move phase (same as louvainState.partition)
    refinedPartition map[NodeID]int  // after BFS split; used for aggregation
    commStr          map[int]float64 // community strengths (same as louvainState.commStr)
    rng              *rand.Rand
}
```

Initialization mirrors `newLouvainState`: each node starts in its own singleton community.

### Pattern 2: Leiden Detect — Overall Flow

```go
func (d *leidenDetector) Detect(g *Graph) (CommunityResult, error) {
    // Guard clauses — identical to Louvain
    // ...

    // Per-iteration loop
    for {
        // Phase 1: local move (reuse Louvain phase1 with leidenState fields)
        ls := &louvainState{partition: state.partition, commStr: state.commStr, rng: state.rng}
        moves := phase1(currentGraph, ls, resolution, m)
        state.partition = ls.partition
        state.commStr = ls.commStr

        // Phase 2 (Leiden-specific): BFS refinement
        state.refinedPartition = refinePartition(currentGraph, state.partition)

        // Track best Q (use refinedPartition for reconstruction)
        candidatePartition := reconstructPartition(origNodes, nodeMapping, state.refinedPartition)
        // ... bestQ tracking same as Louvain ...

        if moves == 0 { break }

        // Phase 3: aggregate using refined partition
        newGraph, superToRep := buildSupergraph(currentGraph, state.refinedPartition)
        // ... nodeMapping update (same pattern as Louvain, but using refinedPartition) ...

        currentGraph = newGraph
    }
}
```

### Pattern 3: BFS Refinement Function

```go
// refinePartition returns a new partition where each connected component
// within every community becomes its own community.
// Self-loops are skipped during BFS (they don't contribute to connectivity).
func refinePartition(g *Graph, partition map[NodeID]int) map[NodeID]int {
    // Group nodes by community
    commNodes := make(map[int][]NodeID)
    for n, c := range partition {
        commNodes[c] = append(commNodes[c], n)
    }

    refined := make(map[NodeID]int, len(partition))
    nextComm := 0

    // Sort community IDs for deterministic output
    // (insertion sort on small list of community IDs)
    for _, comm := range sortedKeys(commNodes) {
        nodes := commNodes[comm]
        // Build node-set for this community (for O(1) neighbor filtering)
        inComm := make(map[NodeID]struct{}, len(nodes))
        for _, n := range nodes {
            inComm[n] = struct{}{}
        }

        visited := make(map[NodeID]bool, len(nodes))
        for _, start := range nodes {
            if visited[start] {
                continue
            }
            // BFS from start, only traverse edges within this community
            queue := []NodeID{start}
            visited[start] = true
            for len(queue) > 0 {
                cur := queue[0]
                queue = queue[1:]
                refined[cur] = nextComm
                for _, e := range g.Neighbors(cur) {
                    if e.To == cur { continue } // skip self-loops
                    if _, ok := inComm[e.To]; !ok { continue } // skip cross-community
                    if !visited[e.To] {
                        visited[e.To] = true
                        queue = append(queue, e.To)
                    }
                }
            }
            nextComm++
        }
    }
    return refined
}
```

**Key insight:** After `refinePartition`, the `refinedPartition` may have more communities than `partition` (disconnected communities were split). For a well-connected graph like Karate Club, most communities will remain intact. The refined partition is then fed into `buildSupergraph` and used for `nodeMapping` updates.

### Pattern 4: nodeMapping Update with Refined Partition

After `buildSupergraph(currentGraph, state.refinedPartition)`, the `nodeMapping` update must use `refinedPartition` instead of `partition`:

```go
commToNewSuper := make(map[int]NodeID, len(superToRep))
for newSuper, rep := range superToRep {
    comm := state.refinedPartition[rep]  // KEY: use refinedPartition here
    commToNewSuper[comm] = newSuper
}

newMapping := make(map[NodeID]NodeID, len(nodeMapping))
for orig, curSuper := range nodeMapping {
    comm := state.refinedPartition[curSuper]  // KEY: use refinedPartition here
    newMapping[orig] = commToNewSuper[comm]
}
```

### Pattern 5: NMI Helper for Tests

NMI (Normalized Mutual Information) between two partitions p1 and p2:

```go
// nmi computes normalized mutual information between two partitions.
// NMI = 2 * I(X;Y) / (H(X) + H(Y))
// where I(X;Y) = H(X) + H(Y) - H(X,Y)
func nmi(p1, p2 map[NodeID]int) float64 {
    n := float64(len(p1))
    if n == 0 { return 0 }

    // Count contingency table
    joint := make(map[[2]int]float64)
    cnt1 := make(map[int]float64)
    cnt2 := make(map[int]float64)
    for node, c1 := range p1 {
        c2 := p2[node]
        joint[[2]int{c1, c2}]++
        cnt1[c1]++
        cnt2[c2]++
    }

    // H(X) + H(Y) - H(X,Y) = I(X;Y)
    hx, hy, hxy := 0.0, 0.0, 0.0
    for _, count := range cnt1 {
        p := count / n
        hx -= p * math.Log2(p)
    }
    for _, count := range cnt2 {
        p := count / n
        hy -= p * math.Log2(p)
    }
    for _, count := range joint {
        p := count / n
        hxy -= p * math.Log2(p)
    }
    mi := hx + hy - hxy
    denom := hx + hy
    if denom == 0 { return 1.0 } // identical single-community partitions
    return 2.0 * mi / denom
}
```

**Ground truth encoding:** `testdata.KarateClubPartition` is `map[int]int`. Tests need to convert it to `map[NodeID]int` for NMI comparison:

```go
gt := make(map[NodeID]int, len(testdata.KarateClubPartition))
for k, v := range testdata.KarateClubPartition {
    gt[NodeID(k)] = v
}
```

### Anti-Patterns to Avoid

- **Using raw `partition` for aggregation:** Always use `refinedPartition` for `buildSupergraph` and `nodeMapping` updates; using the wrong partition defeats Leiden's correctness guarantee.
- **Running BFS with cross-community edges:** BFS must filter to only edges where both endpoints share the same community in `partition`; crossing community boundaries would merge unrelated nodes.
- **Skipping self-loops check in BFS:** Self-loops (`e.To == cur`) must be skipped; they don't provide connectivity to other nodes.
- **Forgetting isolated nodes in BFS:** Nodes with no intra-community neighbors are valid singletons — BFS from them terminates immediately and assigns them their own refined community.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Phase 1 local move | Custom ΔQ loop | Existing `phase1` function | Already tested, handles edge cases, deterministic shuffle |
| Supergraph compression | Custom edge accumulation | Existing `buildSupergraph` | Handles self-loops, canonicalized edge keys, isolated nodes |
| Partition normalization | Custom renaming | Existing `normalizePartition` | Deterministic 0-indexed output required |
| NMI formula | External library | Inline `nmi()` test helper | Test-only; no public API surface needed; formula is simple |
| Connectivity check | Union-Find | BFS within community | BFS is simpler for small communities; Union-Find adds code complexity for marginal gain |

**Key insight:** The Leiden-specific work is only the BFS refinement pass (~30 lines). Everything else is reused directly from Louvain.

## Common Pitfalls

### Pitfall 1: nodeMapping update after refined aggregation

**What goes wrong:** After `buildSupergraph(g, refinedPartition)`, the `superToRep` maps new supernodes to representative nodes. The representative's community must be looked up in `refinedPartition`, not `partition`. Using `partition` here gives stale community IDs that don't exist in the new supergraph.

**Why it happens:** Louvain code uses `partition` for the mapping update; Leiden's equivalent step must use `refinedPartition`. Copy-paste from Louvain without changing the partition reference breaks correctness.

**How to avoid:** In the nodeMapping update block, replace every `superPartition[rep]` / `superPartition[curSuper]` reference with `state.refinedPartition[...]`.

**Warning signs:** NMI < 0.5 on Karate Club, or `index out of range` panics from missing community IDs in `commToNewSuper`.

### Pitfall 2: BFS treats cross-community edges as intra-community

**What goes wrong:** If BFS does not filter by `inComm` set, it will follow edges into neighboring communities, merging them into the same connected component.

**Why it happens:** `g.Neighbors(n)` returns all edges, not just intra-community ones.

**How to avoid:** Build an `inComm map[NodeID]struct{}` for the current community before BFS. In the BFS loop, only enqueue `e.To` if `e.To` is in `inComm`.

**Warning signs:** `TestLeidenConnectedCommunities` passes trivially (single community for all nodes) because all nodes get merged into one component.

### Pitfall 3: `phase1` requires `*louvainState` — field layout must match

**What goes wrong:** If `leidenState` does not expose `partition`, `commStr`, and `rng` in a way that can be passed to `phase1`, compilation fails or a wrapper must be created.

**Why it happens:** `phase1` is typed to `*louvainState`. Go does not allow implicit interface satisfaction for structs.

**How to avoid:** Create a `*louvainState` wrapper inline when calling `phase1`:
```go
ls := &louvainState{partition: state.partition, commStr: state.commStr, rng: state.rng}
moves := phase1(currentGraph, ls, resolution, m)
state.partition = ls.partition
state.commStr = ls.commStr
```
After `phase1`, copy back any mutated fields.

**Warning signs:** Compilation error `cannot use state (type *leidenState) as type *louvainState`.

### Pitfall 4: NMI is partition-label-agnostic — compare community structure, not IDs

**What goes wrong:** Ground truth uses community IDs `{0, 1}`. Leiden output uses different IDs. Direct partition equality fails even when structure is correct.

**Why it happens:** Community IDs are arbitrary labels; NMI measures structural agreement regardless of label assignment.

**How to avoid:** Use the `nmi()` helper; it computes mutual information over the contingency table, which is label-independent.

**Warning signs:** NMI = 0 or NaN when visually the partitions look correct.

### Pitfall 5: Best-Q tracking must use refinedPartition for reconstruction

**What goes wrong:** When reconstructing `candidatePartition` for Q tracking, using raw `partition` instead of `refinedPartition` gives an incorrect Q estimate (disconnected communities may have inflated Q).

**Why it happens:** Leiden's output is the refined partition, not the local-move partition.

**How to avoid:** `reconstructPartition(origNodes, nodeMapping, state.refinedPartition)` for Q tracking.

## Code Examples

### Full BFS Refinement (authoritative pattern)

```go
// Source: designed per Traag et al. (2019) Section 3 — refinement step
// refinePartition splits each community into its internally-connected components.
func refinePartition(g *Graph, partition map[NodeID]int) map[NodeID]int {
    // Group nodes by community (sorted for determinism)
    commNodes := make(map[int][]NodeID)
    for n, c := range partition {
        commNodes[c] = append(commNodes[c], n)
    }

    // Collect and sort community IDs
    commIDs := make([]int, 0, len(commNodes))
    for c := range commNodes {
        commIDs = append(commIDs, c)
    }
    for i := 1; i < len(commIDs); i++ {
        for j := i; j > 0 && commIDs[j] < commIDs[j-1]; j-- {
            commIDs[j], commIDs[j-1] = commIDs[j-1], commIDs[j]
        }
    }

    refined := make(map[NodeID]int, len(partition))
    nextID := 0

    for _, comm := range commIDs {
        nodes := commNodes[comm]
        inComm := make(map[NodeID]struct{}, len(nodes))
        for _, n := range nodes {
            inComm[n] = struct{}{}
        }
        visited := make(map[NodeID]bool, len(nodes))
        for _, start := range nodes {
            if visited[start] {
                continue
            }
            queue := []NodeID{start}
            visited[start] = true
            for len(queue) > 0 {
                cur := queue[0]
                queue = queue[1:]
                refined[cur] = nextID
                for _, e := range g.Neighbors(cur) {
                    if e.To == cur {
                        continue // self-loop
                    }
                    if _, ok := inComm[e.To]; !ok {
                        continue // cross-community edge
                    }
                    if !visited[e.To] {
                        visited[e.To] = true
                        queue = append(queue, e.To)
                    }
                }
            }
            nextID++
        }
    }
    return refined
}
```

### Test Structure for leiden_test.go

```go
// Source: mirrors louvain_test.go pattern
func TestLeidenKarateClubAccuracy(t *testing.T) {
    g := buildKarateClubLeiden() // same as buildKarateClubLouvain
    det := NewLeiden(LeidenOptions{Seed: 42})
    res, err := det.Detect(g)
    if err != nil { t.Fatalf(...) }
    if res.Modularity <= 0.35 { t.Errorf(...) }

    gt := make(map[NodeID]int, len(testdata.KarateClubPartition))
    for k, v := range testdata.KarateClubPartition {
        gt[NodeID(k)] = v
    }
    score := nmi(res.Partition, gt)
    if score < 0.7 { t.Errorf("NMI = %.4f, want >= 0.7", score) }
}

func TestLeidenConnectedCommunities(t *testing.T) {
    // Run Detect; for each community, extract subgraph and verify connectivity via BFS
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Louvain (may produce disconnected communities) | Leiden with BFS refinement | Traag et al. 2019 | Guarantees internally-connected communities; usually equal or better NMI |
| Manual connectivity checks post-hoc | Refinement phase integrated per-iteration | Traag et al. 2019 | Communities are always valid at aggregation time |

## Open Questions

1. **`phase1` call convention** — `phase1` accepts `*louvainState`. The cleanest bridge is to construct a throw-away `*louvainState` each iteration, call `phase1`, then copy `partition` and `commStr` back into `leidenState`. This is O(n) overhead per iteration but negligible for Karate Club sizes.
   - What's unclear: Whether to embed `*louvainState` inside `leidenState` (avoids copy) or use the wrapper approach (clearer separation).
   - Recommendation: Use the inline wrapper pattern; it avoids coupling the two state types and keeps `leidenState` self-contained.

2. **Best-Q tracking with refined vs. raw partition** — Louvain tracks best Q against `state.partition` (post-phase1). For Leiden, Q should be measured against `refinedPartition` since that is what aggregation uses.
   - What's unclear: Whether measuring Q on `refinedPartition` vs. `partition` produces meaningfully different values in practice.
   - Recommendation: Use `refinedPartition` for Q tracking — it reflects the actual structure being aggregated.

## Environment Availability

Step 2.6: SKIPPED (no external dependencies — pure Go, no new tools required beyond existing `go test`)

## Sources

### Primary (HIGH confidence)

- Direct code inspection: `graph/louvain.go`, `graph/louvain_state.go`, `graph/detector.go`, `graph/graph.go` — full function signatures, type shapes, reuse opportunities
- Direct code inspection: `graph/testdata/karate.go` — `KarateClubEdges` (78 edges), `KarateClubPartition` (34-node ground-truth)
- `03-CONTEXT.md` — locked decisions from discuss-phase

### Secondary (MEDIUM confidence)

- [Traag et al. 2019, arXiv:1810.08473](https://arxiv.org/abs/1810.08473) — original Leiden paper: three-phase structure (local move, refinement, aggregation), BFS connectivity guarantee
- [Wikipedia: Leiden algorithm](https://en.wikipedia.org/wiki/Leiden_algorithm) — confirms three-phase structure

### Tertiary (LOW confidence)

- WebSearch: BFS pseudocode details from community implementations — verified against paper description, consistent with locked decision in CONTEXT.md

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all helpers verified by direct code reading
- Architecture patterns: HIGH — BFS pattern derived from locked decisions + paper structure; NMI formula is standard information theory
- Pitfalls: HIGH — derived from direct analysis of Louvain code and Leiden structural differences

**Research date:** 2026-03-29
**Valid until:** 2026-06-29 (stable algorithm domain)
