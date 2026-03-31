# Phase 11: Incremental Recomputation Core — Research

**Researched:** 2026-03-31
**Domain:** Incremental ego-splitting: affected-node scoping, persona graph patching, warm-started global detection, PersonaID disjointness
**Confidence:** HIGH (all findings sourced directly from codebase; no external libraries needed)

---

## Summary

Phase 11 replaces the single `d.Detect(g)` fallback inside `Update()` with an incremental path that (a) scopes ego-net recomputation to affected nodes only, (b) patches the persona graph incrementally instead of rebuilding it from scratch, (c) warm-starts the global Louvain/Leiden pass from the prior community partition, and (d) allocates new PersonaIDs above `maxExistingPersonaID` to preserve the disjoint PersonaID invariant.

All four requirements (ONLINE-05, ONLINE-06, ONLINE-07, ONLINE-11) can be satisfied by augmenting `buildPersonaGraph` with an incremental variant and threading the prior state through `Update()`. No new external dependencies are needed. All required building blocks — `buildEgoNet`, `buildPersonaGraph`, `mapPersonasToOriginal`, `NewLouvain` with `InitialPartition`, and the existing `OverlappingCommunityResult` structure — already exist in the package.

The single biggest design decision is whether to expose a new `buildPersonaGraphIncremental` function or to inline the incremental path inside `Update()`. Given the complexity of the persona graph wiring logic (edge deduplication, cross-lookup `partitions[u][v]`), a dedicated helper function is the safer, more testable choice.

**Primary recommendation:** Implement `buildPersonaGraphIncremental(g, affected, priorPersonaOf, priorInverseMap, priorPartitions, localDetector)` that rebuilds ego-nets and personas only for the `affected` node set, carries over persona data for all other nodes from the prior state, then re-wires all edges incident to any affected node in the persona graph.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ONLINE-05 | `Update()` recomputes ego-nets only for affected nodes (new nodes + all neighbors of edge endpoints) | `buildEgoNet(g, v)` exists; affected-node set is straightforward set union; only those nodes need `localDetector.Detect` called |
| ONLINE-06 | `Update()` patches persona graph incrementally — unaffected nodes' personas carried over from prior state | `buildPersonaGraph` returns `personaOf` and `inverseMap`; these maps can be copied from prior result and selectively overwritten for affected nodes; edge rewiring must cover all edges incident to affected personas |
| ONLINE-07 | `Update()` warm-starts global detection from prior result's community partition via `InitialPartition` | `LouvainOptions.InitialPartition map[NodeID]int` exists and is threaded through `state.reset()`; `GlobalDetector` options must be reconstructed with `InitialPartition` set to `prior` persona-level partition |
| ONLINE-11 | New PersonaIDs never collide with original NodeID space — allocated from `maxExistingPersonaID + 1` | `buildPersonaGraph` already tracks `nextPersona = maxNodeID + 1`; incremental path must read `maxExistingPersonaID` from `priorInverseMap` keys, not from `g.Nodes()` alone |
</phase_requirements>

---

## Project Constraints (from CLAUDE.md)

- Single `package graph` — no sub-packages.
- Stdlib only — no new external imports.
- Do NOT modify `Detect()` behavior — `Update()` is purely additive.
- All code reviews must be delegated to a subagent before marking complete.
- GSD workflow required; commit `.planning/` artifacts.

---

## Standard Stack

### Core (all already in package — nothing to install)

| Symbol | File | Purpose |
|--------|------|---------|
| `buildEgoNet(g *Graph, v NodeID) *Graph` | `ego_splitting.go` | Builds the ego-net for a single node |
| `buildPersonaGraph(g, localDetector)` | `ego_splitting.go` | Full persona graph construction — incremental variant derives from this |
| `mapPersonasToOriginal(partition, inverseMap)` | `ego_splitting.go` | Converts global persona partition → original node overlapping membership |
| `NewLouvain(LouvainOptions{InitialPartition: ...})` | `detector.go`, `louvain.go` | Warm-starts global detection from prior partition |
| `LouvainOptions.InitialPartition map[NodeID]int` | `detector.go` | Carries prior partition into `state.reset()` on first pass only |
| `CommunityDetector` interface | `detector.go` | Both `LocalDetector` and `GlobalDetector` implement this |

---

## Architecture Patterns

### Recommended Project Structure (no new files required)

All new code goes into `graph/ego_splitting.go` (implementation) and `graph/ego_splitting_test.go` (tests). No new files are needed for this phase.

### Pattern 1: Affected-Node Scoping (ONLINE-05)

**What:** Compute the set of nodes whose ego-nets must be recomputed. For an `AddedNodes` + `AddedEdges` delta, affected = new nodes ∪ {all neighbors of each edge endpoint in the updated graph}.

**When to use:** Every non-empty delta path in `Update()`.

**Logic:**
```go
// Affected node set — O(delta * avg_degree)
affected := make(map[NodeID]struct{})
for _, n := range delta.AddedNodes {
    affected[n] = struct{}{}
}
for _, e := range delta.AddedEdges {
    affected[e.From] = struct{}{}
    affected[e.To] = struct{}{}
    for _, nb := range g.Neighbors(e.From) {
        affected[nb.To] = struct{}{}
    }
    for _, nb := range g.Neighbors(e.To) {
        affected[nb.To] = struct{}{}
    }
}
```

**Key insight:** Neighbors are queried on the already-updated `g` (the caller has already applied the delta to the graph before calling `Update()`). This is correct: the ego-net of a neighbor of a new edge's endpoint changes because that neighbor's ego-net now includes the new edge.

### Pattern 2: Incremental Persona Graph Build (ONLINE-06)

**What:** Copy `personaOf`, `inverseMap`, and `partitions` from the prior run. For each node in `affected`, delete its old persona entries and rebuild fresh ones. Then re-wire ALL edges incident to any affected persona node.

**Key structures to carry over:**
- `personaOf map[NodeID]map[int]NodeID` — original node → community → PersonaID
- `inverseMap map[NodeID]NodeID` — PersonaID → original NodeID
- `partitions map[NodeID]map[NodeID]int` — ego-net partition per node (needed for edge wiring)

**Critical detail:** When removing affected nodes' old personas, those PersonaIDs must be deleted from `inverseMap` and the persona graph. When re-wiring edges, ALL edges in `g` that touch any affected persona must be re-processed — not just the new edges — because the partition assignments may have changed.

**Persona graph surgery — only affected nodes:**
```go
// 1. Delete old persona entries for affected nodes
for v := range affected {
    for _, oldPersona := range priorPersonaOf[v] {
        delete(priorInverseMap, oldPersona)
        personaGraph.RemoveNode(oldPersona) // if Graph gains RemoveNode, else rebuild persona graph
    }
    delete(priorPersonaOf, v)
    delete(priorPartitions, v)
}
```

**IMPORTANT — Graph has no `RemoveNode`:** `graph.go` does not expose `RemoveNode` or `RemoveEdge`. The persona graph must be rebuilt from scratch using the patched `personaOf`/`inverseMap`/`partitions` data rather than surgically removing nodes from an existing persona graph. This is still O(affected * avg_degree) for node creation and O(|edges incident to affected|) for wiring, vs O(n * avg_degree) for full rebuild.

### Pattern 3: Warm-Start Global Detection (ONLINE-07)

**What:** After the incremental persona graph is built, convert the prior `OverlappingCommunityResult`'s persona-level partition to an `InitialPartition` map and pass it to `GlobalDetector`.

**How the prior partition is encoded:** `Detect()` calls `d.opts.GlobalDetector.Detect(personaGraph)` which returns a `CommunityResult{Partition map[NodeID]int}`. This persona-level partition is what goes into `InitialPartition`. The `OverlappingCommunityResult` does NOT directly store the persona-level partition — only the final `NodeCommunities` map is stored.

**Implication for ONLINE-07:** `Update()` needs access to the intermediate persona-level partition from the prior run. Two approaches:

| Approach | Pros | Cons |
|----------|------|------|
| A. Store persona partition inside `OverlappingCommunityResult` as a new unexported field | Clean; callers don't see it; `Update()` can always use it | Mutates the existing result struct |
| B. Carry `priorPersonaPartition map[NodeID]int` as a separate parameter to the incremental helper | No struct change; pure function | Requires reconstructing it somehow |
| C. Reconstruct persona partition from `NodeCommunities` + `inverseMap` | No new fields needed | Requires storing `personaOf`/`inverseMap` from prior run too |

**Recommended: Approach A** — add unexported fields to `OverlappingCommunityResult` to hold the intermediate state needed by `Update()`:

```go
type OverlappingCommunityResult struct {
    Communities     [][]NodeID
    NodeCommunities map[NodeID][]int
    // unexported fields for incremental Update() — zero values yield cold-start fallback
    personaOf      map[NodeID]map[int]NodeID // original node -> community -> PersonaID
    inverseMap     map[NodeID]NodeID          // PersonaID -> original NodeID
    partitions     map[NodeID]map[NodeID]int  // ego-net partitions per node
    personaPartition map[NodeID]int           // persona-level global partition
}
```

`Detect()` populates these fields before returning. `Update()` reads them. If any are nil (e.g., result came from old code or zero-value), fall back to `d.Detect(g)`. This preserves full backward compatibility — the public API is unchanged.

### Pattern 4: PersonaID Allocation (ONLINE-11)

**What:** New personas for affected nodes must not collide with either (a) original NodeIDs in `g` or (b) existing PersonaIDs in `priorInverseMap`.

**Current `buildPersonaGraph` logic:** `nextPersona = maxNodeID + 1` where `maxNodeID` is computed from `g.Nodes()`. This is correct for a full rebuild but wrong for incremental updates — after the first `Update()`, new PersonaIDs must start above the maximum PersonaID already allocated.

**Correct incremental formula:**
```go
// Find max existing PersonaID across all prior personas
maxExistingPersonaID := NodeID(0)
for personaID := range priorInverseMap {
    if personaID > maxExistingPersonaID {
        maxExistingPersonaID = personaID
    }
}
// Also ensure we're above all original NodeIDs
for _, n := range g.Nodes() {
    if n > maxExistingPersonaID {
        maxExistingPersonaID = n
    }
}
nextPersona := maxExistingPersonaID + 1
```

**Why this matters:** If `Detect()` on a 34-node Karate Club graph allocates PersonaIDs 34..80, then `Update()` adds node 35 (which happens to be > 34 but a legitimate original NodeID after a few additions), the next `buildPersonaGraph` call could reuse IDs already in `priorInverseMap` if it only looks at `g.Nodes()`.

### Anti-Patterns to Avoid

- **Iterating all n nodes for ego-net recomputation:** This is the Phase 10 placeholder path. Phase 11 must scope to `affected` only (ONLINE-05).
- **Rebuilding partitions from `g.Nodes()` max instead of `priorInverseMap` max:** Leads to PersonaID collisions on graphs that have grown beyond their initial size.
- **Carrying over the persona-level partition from `prior.NodeCommunities`:** `NodeCommunities` is indexed by original NodeIDs, not PersonaIDs. The warm-start partition for Louvain must use PersonaIDs as keys.
- **Re-wiring only new edges in the persona graph:** When an affected node's ego-net partition changes (new local community assignment), ALL edges incident to that node must be re-wired in the persona graph, not just the newly added edges.
- **Calling `d.opts.GlobalDetector.Detect(personaGraph)` without propagating `InitialPartition`:** `GlobalDetector` is a `CommunityDetector` interface — `louvainDetector` — its options are set at construction time. To warm-start, reconstruct a new `louvainDetector` with `InitialPartition` set, OR use `NewLouvain(LouvainOptions{..., InitialPartition: priorPersonaPartition})` inline. The stored `d.opts.GlobalDetector` must NOT be mutated (it is shared and pool-reused).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Warm-start Louvain | Custom partition seeding | `NewLouvain(LouvainOptions{InitialPartition: ...})` | Already implemented; `state.reset()` handles new-node singletons and compaction automatically |
| Ego-net construction | Custom subgraph builder | `buildEgoNet(g, v)` | Already handles exclusion of ego node (EGO-CRIT-01) |
| PersonaID → original mapping | Custom lookup table | `inverseMap map[NodeID]NodeID` | Already built by `buildPersonaGraph`; incremental path copies and patches it |
| Edge deduplication in persona graph | Custom dedup | Canonical `[2]NodeID{lo, hi}` seen-map pattern | Already used in `buildPersonaGraph` — copy the same pattern |

---

## Common Pitfalls

### Pitfall 1: Mutating GlobalDetector options
**What goes wrong:** Setting `d.opts.GlobalDetector.(*louvainDetector).opts.InitialPartition = ...` mutates shared state across concurrent calls.
**Why it happens:** `louvainDetector` is a struct; its options are value-typed, but the `InitialPartition` map is a reference.
**How to avoid:** Construct a new `CommunityDetector` inline for the global detection step in `Update()`:
```go
warmDetector := NewLouvain(LouvainOptions{
    Resolution:       d.opts.GlobalDetector.(*louvainDetector).opts.Resolution,
    Seed:             d.opts.GlobalDetector.(*louvainDetector).opts.Seed,
    InitialPartition: priorPersonaPartition,
})
globalResult, err := warmDetector.Detect(personaGraph)
```
Alternatively, accept `CommunityDetector` as a parameter to avoid the type assertion.

**Better approach:** Extract the global detector options via a helper, or accept that `Update()` always constructs a fresh warm detector — the cost is trivial (one `NewLouvain` call).

### Pitfall 2: PersonaID collision after multiple Updates
**What goes wrong:** Each call to `Update()` that adds nodes pushes `maxNodeID` up, but if `nextPersona` is computed as `maxNodeID + 1` from `g.Nodes()`, and a prior `Update()` already allocated PersonaIDs in the range `[maxNodeID+1, X]`, the new call can reassign already-used PersonaIDs.
**How to avoid:** Always compute `nextPersona` as `max(g.Nodes()) + 1` AND `max(priorInverseMap keys) + 1`, taking the larger value.

### Pitfall 3: Missing edge re-wiring for unaffected neighbors
**What goes wrong:** When node `v` is in `affected`, its ego-net partition changes. Node `u` (a neighbor of `v`, not in `affected`) appears as a neighbor in `v`'s ego-net. The edge between persona(u) and persona(v) must be updated, but `u` is not in `affected`, so its persona is carried over.
**Why it happens:** Edge wiring depends on BOTH endpoints' partition assignments. `commOfVinGu` in `buildPersonaGraph` reads `partitions[u][v]` — which is the ego-net partition of v seen from u. If u is not in affected, `partitions[u]` is unchanged, so the edge wiring for u's side is still valid. However, `partitions[v]` HAS changed, so ALL edges incident to v must be re-wired.
**How to avoid:** After computing new personas for affected nodes, delete and rebuild ALL persona graph edges where at least one endpoint is an affected original node. Iterate over `g.Neighbors(v)` for every `v` in affected.

### Pitfall 4: Prior result with nil unexported fields (cold Update() call)
**What goes wrong:** Caller passes a prior result that was constructed by old code or is a zero value — unexported fields (`personaOf`, `inverseMap`, etc.) are nil.
**How to avoid:** At the top of the incremental path, check:
```go
if prior.personaOf == nil || prior.inverseMap == nil || prior.partitions == nil {
    return d.Detect(g) // graceful fallback to full recompute
}
```

### Pitfall 5: Warm-start partition keys are PersonaIDs, not original NodeIDs
**What goes wrong:** Passing `prior.NodeCommunities` (keyed by original NodeIDs) as `InitialPartition` to `NewLouvain`. Louvain expects PersonaID keys that match the persona graph's node IDs.
**How to avoid:** Store `personaPartition map[NodeID]int` (the raw output of `GlobalDetector.Detect`) in the prior result. When constructing the warm-started detector, pass this map directly. For affected personas (whose PersonaIDs changed), insert new entries with singleton community IDs.

---

## Code Examples

### Q1: What does Update() need to call at its core?

Current Phase 10 stub (to replace):
```go
// TODO(Phase 11): replace with incremental recomputation.
return d.Detect(g)
```

Phase 11 replacement sketch:
```go
// 1. Compute affected node set
affected := computeAffected(g, delta)

// 2. Build incremental persona graph, reusing prior state
personaGraph, newPersonaOf, newInverseMap, newPersonaPartition, err :=
    buildPersonaGraphIncremental(g, affected, prior, d.opts.LocalDetector)
if err != nil {
    return OverlappingCommunityResult{}, err
}

// 3. Warm-start global detection
warmGlobal := newGlobalDetectorWithWarmStart(d.opts, newPersonaPartition)
globalResult, err := warmGlobal.Detect(personaGraph)
if err != nil {
    return OverlappingCommunityResult{}, err
}

// 4. Map personas back to original nodes (same as Detect)
nodeCommunities := mapPersonasToOriginal(globalResult.Partition, newInverseMap)
// ... dedup, compact, build result (identical to Detect steps 4-5)
result := buildResult(nodeCommunities)
result.personaOf = newPersonaOf
result.inverseMap = newInverseMap
result.partitions = newPartitions
result.personaPartition = globalResult.Partition
return result, nil
```

### Q2: How does InitialPartition warm-start work in louvain_state.go?

Source: `graph/louvain_state.go` lines 49-116

```go
func (st *louvainState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
    // ...
    if initialPartition == nil {
        // Cold start: singleton assignment
        for i, n := range nodes {
            st.partition[n] = i
            st.commStr[i] = g.Strength(n)
        }
        return
    }
    // Warm start:
    // 1. Assign known nodes their prior community
    // 2. New nodes (not in initialPartition) get fresh singleton communities
    // 3. Compact to 0-indexed contiguous IDs
    // 4. Rebuild commStr from CURRENT graph strengths
}
```

Key facts:
- `InitialPartition` keys are **PersonaIDs** (node IDs in the persona graph).
- New nodes absent from `InitialPartition` automatically get fresh singleton communities.
- `commStr` is always rebuilt from the CURRENT graph — no stale strength data (Phase 05 decision).
- The warm-start only applies on `firstPass` in the Louvain outer loop; supergraph passes always cold-reset.

### Q3: buildPersonaGraph — what data is available for incremental reuse?

Source: `graph/ego_splitting.go` lines 215-325

`buildPersonaGraph` returns:
1. `personaGraph *Graph` — persona graph (must be rebuilt each time since no RemoveNode)
2. `personaOf map[NodeID]map[int]NodeID` — original node → community → PersonaID
3. `inverseMap map[NodeID]NodeID` — PersonaID → original NodeID

Internally it also builds:
- `partitions map[NodeID]map[NodeID]int` — ego-net partition per node (neighbor → community ID in G_v)

`partitions` is NOT returned but is needed for edge wiring in the incremental path. The incremental helper must receive (and return) `partitions` alongside `personaOf` and `inverseMap`.

This means `OverlappingCommunityResult` needs to carry `partitions` as an unexported field.

### Q4: Reconstructing the persona-level warm-start partition for new personas

After recomputing ego-nets for affected nodes, new personas are allocated. Their community assignments in the warm-start should default to singleton (handled automatically by Louvain when a PersonaID is absent from `InitialPartition`). The prior personas for unaffected nodes retain their prior community assignments.

```go
// Build warm-start partition for persona graph:
// Start with prior persona partition, remove deleted personas, new ones absent = singleton
warmPartition := make(map[NodeID]int, len(priorPersonaPartition))
for personaID, comm := range prior.personaPartition {
    if _, stillExists := newInverseMap[personaID]; stillExists {
        warmPartition[personaID] = comm
    }
    // deleted personas (affected nodes' old personas) are simply omitted
}
// New personas for affected nodes are absent from warmPartition
// → Louvain assigns them singleton communities automatically
```

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing (`go test`) |
| Config file | none (standard Go toolchain) |
| Quick run command | `go test ./graph/... -run "TestUpdate" -v -count=1` |
| Full suite command | `go test ./graph/... -count=1 -race` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ONLINE-05 | Only affected nodes' ego-nets recomputed | unit (spy via side-channel counter or indirect assertion) | `go test ./graph/... -run "TestUpdate_AffectedNodesOnly" -v` | No — Wave 0 |
| ONLINE-06 | Unaffected nodes' personas carried over | unit (check PersonaIDs match prior for unaffected nodes) | `go test ./graph/... -run "TestUpdate_UnaffectedPersonasCarriedOver" -v` | No — Wave 0 |
| ONLINE-07 | Warm-start path taken for global detection | integration (verify Q(warm) >= Q(cold_perturbed) or lower pass count) | `go test ./graph/... -run "TestUpdate_WarmStartGlobalDetection" -v` | No — Wave 0 |
| ONLINE-11 | PersonaIDs from Update never collide with NodeID space | unit (assert all new PersonaIDs > max(g.Nodes()) AND > max(prior PersonaIDs)) | `go test ./graph/... -run "TestUpdate_PersonaIDDisjoint" -v` | No — Wave 0 |

### Testing "only affected ego-nets recomputed" (ONLINE-05) without exposing internals

Since `buildPersonaGraph`/`buildEgoNet` are package-private, the test can use the following indirect approach:
- Add a node to a graph where most nodes are not neighbors of the new node.
- Assert that PersonaIDs for unaffected nodes are identical between `prior.personaOf` and `result.personaOf` (both stored as unexported fields, accessible within `package graph` tests).
- This is a white-box test inside `package graph` — the `_test.go` files are in `package graph`, so they can access unexported fields.

Alternatively, a counter-based spy can be injected by wrapping `LocalDetector`:
```go
type countingDetector struct {
    inner CommunityDetector
    count int
}
func (c *countingDetector) Detect(g *Graph) (CommunityResult, error) {
    c.count++
    return c.inner.Detect(g)
}
```
Pass `countingDetector` as `LocalDetector`; assert that `count == len(affected)` after `Update()`.

### Wave 0 Gaps

- [ ] `graph/ego_splitting_test.go` — add `TestUpdate_AffectedNodesOnly` (REQ ONLINE-05)
- [ ] `graph/ego_splitting_test.go` — add `TestUpdate_UnaffectedPersonasCarriedOver` (REQ ONLINE-06)
- [ ] `graph/ego_splitting_test.go` — add `TestUpdate_WarmStartGlobalDetection` (REQ ONLINE-07)
- [ ] `graph/ego_splitting_test.go` — add `TestUpdate_PersonaIDDisjoint` (REQ ONLINE-11)

No new test files needed — all tests extend the existing `ego_splitting_test.go`.

---

## Open Questions

1. **GlobalDetector type assertion**
   - What we know: `d.opts.GlobalDetector` is a `CommunityDetector` interface; at runtime it is a `*louvainDetector` or `*leidenDetector`.
   - What's unclear: Phase 11 needs to reconstruct a warm-started detector with the same seed/resolution but a new `InitialPartition`. A type switch is needed to extract options.
   - Recommendation: Define a helper `warmStartedDetector(d CommunityDetector, partition map[NodeID]int) CommunityDetector` with a type switch on `*louvainDetector` and `*leidenDetector`. This keeps `Update()` clean and avoids coupling.

2. **`OverlappingCommunityResult` struct mutation**
   - What we know: Adding unexported fields changes the struct but is backward-compatible for external callers (zero values = graceful fallback to full Detect).
   - What's unclear: Whether ONLINE-12/ONLINE-13 (Phase 13) will require copying this struct — unexported fields must also be copied.
   - Recommendation: Acceptable for Phase 11. Document the unexported fields clearly. Phase 13 will need to account for them when implementing result invariant checks.

3. **Persona graph "remove node" gap**
   - What we know: `Graph` has no `RemoveNode` method. The persona graph for affected nodes cannot be surgically patched.
   - What's unclear: Whether rebuilding the persona graph from the patched `personaOf`/`inverseMap` data is fast enough to satisfy the ONLINE-08 benchmark in Phase 12.
   - Recommendation: Rebuild the persona graph from scratch using the patched data structures (not from calling `buildPersonaGraph` on the full graph). This is O(|personas| + |persona edges incident to affected|), which is a small constant of the full O(n * avg_degree) rebuild. Acceptable for Phase 11; profile in Phase 12.

---

## Environment Availability

Step 2.6: SKIPPED — Phase 11 is a pure Go code change with no external dependencies beyond the existing Go toolchain.

---

## Sources

### Primary (HIGH confidence)

All findings sourced directly from the codebase — no external libraries needed.

- `graph/ego_splitting.go` — `buildPersonaGraph`, `mapPersonasToOriginal`, `Update()` stub, `buildEgoNet`
- `graph/louvain_state.go` lines 49-116 — `state.reset()` warm-start implementation
- `graph/detector.go` — `LouvainOptions.InitialPartition`, `CommunityDetector` interface
- `graph/graph.go` — `Subgraph`, `AddNode`, `AddEdge`, `Neighbors`, `Nodes` — confirmed no `RemoveNode`/`RemoveEdge`
- `graph/louvain.go` lines 81-88 — `firstPass` guard: `InitialPartition` only applied on first pass, supergraph passes always cold
- `.planning/STATE.md` — prior decisions (EGO-CRIT-02 PersonaID invariant, Phase 05 warm-start decisions)
- `.planning/REQUIREMENTS.md` — ONLINE-05 through ONLINE-11 specifications

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all symbols verified in codebase
- Architecture: HIGH — incremental design derived directly from existing `buildPersonaGraph` logic
- Pitfalls: HIGH — sourced from existing code structure (no RemoveNode, type assertion for GlobalDetector, PersonaID allocation logic)

**Research date:** 2026-03-31
**Valid until:** Stable (pure internal codebase research; no external API versioning concerns)
