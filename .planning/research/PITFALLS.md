# Pitfalls Research: Ego Splitting Framework (v1.2 milestone)

**Domain:** Overlapping community detection via Ego Splitting (Epasto et al., KDD 2017) added to existing Go graph library
**Researched:** 2026-03-30
**Confidence:** HIGH (algorithm correctness, Go integration) / MEDIUM (overlapping NMI specifics)

> This file EXTENDS the existing PITFALLS.md for v1.0/v1.1 (Louvain + Leiden).
> It covers pitfalls specific to the Ego Splitting Framework and its integration
> with the `package graph` codebase.

---

## Critical Pitfalls

---

### EGO-CRIT-01: Ego-Net Boundary — Including the Ego Node Itself

**What goes wrong:**
Algorithm 1 builds the ego-net of node `v` as the subgraph induced by `v`'s
*neighbors*, explicitly **excluding** `v` itself (the paper calls this the
"egoless ego-net"). If `v` is included in its own ego-net subgraph before
running internal community detection, the inner detector sees `v` as a hub
with edges to every node, which dominates ΔQ and forces all ego-net neighbors
into a single community. This eliminates the persona splitting that is the
algorithm's entire purpose.

**Why it happens:**
The `Graph.Subgraph(nodeIDs)` method takes a slice of NodeIDs. The natural
implementation passes `g.Neighbors(v)` converted to NodeIDs plus `v` itself
("give me the induced subgraph of v and its neighbors"). Omitting `v` is a
paper-specific requirement that is easy to miss when reading Algorithm 1
casually.

**How to avoid:**
```go
// CORRECT: ego-net of v excludes v itself
func buildEgoNet(g *Graph, v NodeID) *Graph {
    neighbors := g.Neighbors(v)
    nodeIDs := make([]NodeID, 0, len(neighbors))
    for _, e := range neighbors {
        nodeIDs = append(nodeIDs, e.To)
        // NOTE: do NOT append v here
    }
    return g.Subgraph(nodeIDs)
}
```

**Warning signs:**
- All ego-net community detection results return a single community for every
  ego-net (community count == 1 for most nodes).
- Persona graph has the same number of nodes as the original graph (no splits
  occurred, meaning every node produced exactly one persona).
- `personaCount[v]` equals 1 for all high-degree nodes — high-degree nodes
  almost always have multi-community neighborhoods if the graph has any
  community structure.

**Phase to address:** Phase implementing Algorithm 1 (ego-net construction).

---

### EGO-CRIT-02: Persona NodeID Space Collision

**What goes wrong:**
The persona graph assigns a fresh NodeID to each persona. A naive
implementation reuses original NodeIDs for the first persona of each node
(persona 0 of node `v` gets ID `v`), then assigns IDs starting from
`maxOriginalNodeID + 1` for subsequent personas. This works if original
NodeIDs are dense and bounded. But the existing `NodeID` type is `int` with
no enforced upper bound, and if the original graph contains NodeIDs like
`{0, 1, 1000, 9999}`, the "max + 1" offset strategy silently produces
collisions when `maxID + numPersonas > nextFreeSlot`.

More critically: if the persona NodeID for persona-k-of-node-v accidentally
equals the original NodeID of a different node `w`, the persona-to-original
mapping `personaToOriginal[personaID] = v` will shadow the real node `w`
when recovering overlapping communities.

**Why it happens:**
Developers conflate "persona ID space" with "original ID space". The two must
be completely disjoint namespaces.

**How to avoid:**
Use a monotonically incrementing counter that starts at 0, independent of
original NodeIDs. Maintain an explicit `personaToOriginal map[NodeID]NodeID`
that maps every persona ID to the original node it represents. Never reuse
original NodeIDs in the persona graph.

```go
type personaGraph struct {
    g                *Graph
    personaToOrig    map[NodeID]NodeID   // persona NodeID -> original NodeID
    origToPersonas   map[NodeID][]NodeID // original NodeID -> list of persona NodeIDs
    nextPersonaID    NodeID
}

func (pg *personaGraph) newPersona(orig NodeID) NodeID {
    id := pg.nextPersonaID
    pg.nextPersonaID++
    pg.personaToOrig[id] = orig
    pg.origToPersonas[orig] = append(pg.origToPersonas[orig], id)
    return id
}
```

**Warning signs:**
- `len(personaToOriginal)` does not equal total persona count.
- Two different original nodes map to the same persona ID in `origToPersonas`.
- Recovering communities produces overlapping assignments that reference nodes
  not in the original graph.

**Phase to address:** Phase implementing Algorithm 2 (persona graph construction).

---

### EGO-CRIT-03: Persona Graph Edge Construction — Double-Counting Undirected Edges

**What goes wrong:**
Algorithm 2 adds an edge between persona `p_u` and persona `p_v` in the
persona graph for each edge `(u, v)` in the original graph, where `p_u` is
the persona of `u` that is co-located (in the same community of `u`'s ego-net)
with `v`, and vice versa. For undirected graphs, `(u, v)` appears in both
`g.Neighbors(u)` and `g.Neighbors(v)`. If the implementation iterates all
nodes and all their neighbors, each undirected edge is processed twice,
inserting the persona-graph edge twice. With `Graph.AddEdge` for undirected
graphs, this means the persona graph accumulates 2x edge weight on every edge.

The consequence is that modularity in the persona graph is computed over a
graph with doubled weights, which shifts the null model and changes the
community structure.

**How to avoid:**
Track processed edges with a canonical-key deduplication set (identical to
the approach in `Graph.Subgraph`), or iterate only `u < v` pairs:

```go
seen := make(map[[2]NodeID]struct{})
for _, u := range g.Nodes() {
    for _, e := range g.Neighbors(u) {
        v := e.To
        pu := findPersona(u, v) // persona of u that neighbors v
        pv := findPersona(v, u) // persona of v that neighbors u
        lo, hi := pu, pv
        if lo > hi { lo, hi = hi, lo }
        key := [2]NodeID{lo, hi}
        if _, already := seen[key]; already {
            continue
        }
        seen[key] = struct{}{}
        personaG.AddEdge(pu, pv, e.Weight)
    }
}
```

**Warning signs:**
- `personaGraph.TotalWeight()` is exactly 2x the original graph's `TotalWeight()`.
- Community detection on the persona graph reports higher modularity Q than
  expected for the graph's structure.

**Phase to address:** Phase implementing Algorithm 2 (persona graph construction).

---

### EGO-CRIT-04: Disconnected Ego-Net — Inner Detector Returns Singleton Partition

**What goes wrong:**
When a node `v` has a neighborhood that is entirely disconnected (no edges
among `v`'s neighbors — `v` is a "star center"), the ego-net subgraph has
zero edges. The existing `louvainDetector.Detect` guard returns a singleton
partition when `g.TotalWeight() == 0`: every node in its own community. For a
star-center node with degree `d`, this produces `d` personas — one per
neighbor. This is technically correct per the paper but causes a combinatorial
explosion in the persona graph: a star node with 1000 neighbors produces 1000
personas, each connected to a single edge in the persona graph.

This is not a bug but a performance trap. The persona graph for a
high-degree star node with an empty ego-net will be very large.

**How to avoid:**
Two options, choose one per requirements:
1. **Paper-accurate:** Allow singleton partition. Document that star-topology
   graphs will produce large persona graphs. Add a max-persona cap per node
   as a safety option (`EgoSplittingOptions.MaxPersonasPerNode int`).
2. **Pragmatic:** If ego-net has zero edges, assign all neighbors to one
   community (treat as a single persona for `v`). This deviates from the
   paper but is a common approximation used in practice.

**Warning signs:**
- Persona graph node count >> 2x original node count for graphs with hub nodes.
- Community detection on persona graph takes longer than expected due to size.

**Phase to address:** Phase implementing Algorithm 1 (ego-net construction), with option surfaced in Algorithm 2.

---

### EGO-CRIT-05: Incorrect Persona-to-Community Recovery (Algorithm 3)

**What goes wrong:**
Algorithm 3 runs community detection on the persona graph, then maps persona
communities back to the original graph. Each original node `v` may have
multiple personas, each potentially in a different persona community. The
overlapping community assignment for `v` is the **set** of distinct community
IDs assigned to `v`'s personas.

A common mistake is to only take the *first* or *majority* persona's community,
losing the overlapping structure entirely. The result looks like a valid
partition but is actually a non-overlapping approximation.

A second mistake: the persona community IDs (from `CommunityResult.Partition`)
are 0-indexed contiguous integers local to the persona graph. After community
detection, different personas of the same original node may be in community 2
and community 5. These are the correct overlapping memberships for that node.
Some implementations accidentally renumber communities during recovery and
merge community 2 from one pass with community 2 from another, producing
incorrect assignments.

**How to avoid:**
```go
// OverlappingCommunityResult maps each NodeID to ALL communities it belongs to.
type OverlappingCommunityResult struct {
    Communities map[NodeID][]int // node -> sorted list of community IDs
    Modularity  float64
}

func recoverOverlappingCommunities(
    personaPartition map[NodeID]int,       // persona NodeID -> community ID
    personaToOrig map[NodeID]NodeID,       // persona NodeID -> original NodeID
) OverlappingCommunityResult {
    result := make(map[NodeID][]int)
    for personaID, commID := range personaPartition {
        origID := personaToOrig[personaID]
        result[origID] = appendUnique(result[origID], commID)
    }
    // Sort each node's community list for determinism
    for id := range result {
        sort.Ints(result[id])
    }
    return OverlappingCommunityResult{Communities: result}
}
```

**Warning signs:**
- `OverlappingCommunityResult.Communities` shows every node with exactly one
  community (overlapping detection degenerating to non-overlapping).
- Nodes that were known to bridge communities in the test fixture (e.g.,
  node 0 and node 33 in Karate Club) appear in only one community.

**Phase to address:** Phase implementing Algorithm 3 (community recovery).

---

### EGO-CRIT-06: NMI for Overlapping Communities — Using Standard (Non-Overlapping) NMI

**What goes wrong:**
The existing `nmi()` function in `testhelpers_test.go` computes standard NMI
for non-overlapping partitions (`map[NodeID]int`). Overlapping community NMI
requires a different formulation. The standard NMI is undefined when a node
belongs to multiple communities because mutual information between two
*partitions* assumes each element appears in exactly one class.

If the existing `nmi()` function is reused for overlapping community
validation, the correct approach requires taking one membership per node
(e.g., the first in the sorted list), which discards the overlapping
information and silently measures something different from what is claimed.

**The correct metric for overlapping communities:**
The standard in the literature is the *overlapping NMI* introduced by
McDaid, Greene & Hurley (2011) — also called `NMI_max` or `onmi`. It computes
NMI between two sets of (potentially overlapping) covers using a
per-community entropy formulation:

```
NMI_ovlp(X, Y) = (1/2) * [H(X|Y)/H(X) + H(Y|X)/H(Y)]
```

where `H(X|Y)` is computed by treating each community as a binary membership
vector across all nodes, then finding the best-matching community from `Y` for
each community in `X`.

**How to avoid:**
Implement `overlapNMI(result map[NodeID][]int, groundTruth map[NodeID][]int) float64`
separately from the existing scalar `nmi()`. The implementation requires:
1. Convert each community to a binary membership vector (length = number of nodes).
2. For each community `X_i` in cover X, find the community `Y_j` in cover Y
   that maximises `NMI(X_i, Y_j)` treating them as binary vectors.
3. Average over all communities in both directions.

Do not use `nmi()` for overlapping validation — it will produce misleadingly
high scores when the algorithm degrades to non-overlapping behavior.

**Warning signs:**
- Overlapping NMI score equals standard NMI score exactly — almost never true.
- NMI test passes for a result where every node is in exactly one community.
- Validation test does not explicitly test that at least some nodes appear in
  multiple communities.

**Phase to address:** Phase implementing accuracy validation (Algorithm 3 tests).

---

### EGO-CRIT-07: Inner CommunityDetector Reuse — sync.Pool Contention Under Parallelism

**What goes wrong:**
Algorithm 1 runs a `CommunityDetector.Detect` call on each node's ego-net.
For a graph with N nodes, this is N inner `Detect` calls. If the outer
`EgoSplitting.Detect` parallelises these calls across goroutines (the natural
optimization), all goroutines compete for the same `louvainStatePool`
(a package-level `sync.Pool`). Under high concurrency, `sync.Pool` behaves
correctly — it is goroutine-safe — but there is a subtler issue:

Each `acquireLouvainState` call on a small ego-net (e.g., 5 nodes) gets a
state object sized for a potentially much larger graph from a prior pooled
entry. The `reset()` method calls `clear(st.partition)` which zeroes the map
in O(current_len) — if the previous occupant processed a 10K-node graph, the
map has 10K entries, and clearing it costs O(10K) even though the ego-net
needs only 5 entries. Multiplied across N goroutines, this becomes a
significant overhead.

**How to avoid:**
Two strategies:
1. **Per-ego-net fresh allocation:** For ego-nets smaller than a threshold
   (e.g., < 50 nodes), allocate state directly without the pool. The pool is
   designed for large graph reuse; small graphs are cheap to allocate fresh.
2. **Separate pool per size tier:** Maintain a small-graph pool (< 100 nodes)
   and large-graph pool (>= 100 nodes) to avoid oversized state objects
   serving undersized graphs.

The simplest safe approach: pass `NewLouvain(opts)` (or `NewLeiden(opts)`)
as the inner detector, which already uses `acquireLouvainState` internally.
The pool contention is acceptable if ego-nets are small (the pool will quickly
populate with small-graph states). Profile before optimising.

**Warning signs:**
- `-benchmem` shows O(N * egoNetSize) allocations per `EgoSplitting.Detect`
  call instead of O(N) (pool misses dominating).
- `go test -race` reports data races on `louvainStatePool` — this would
  indicate the pool itself is not being used correctly (should not happen with
  standard `sync.Pool`).

**Phase to address:** Phase implementing parallel ego-net detection (Algorithm 1 parallelisation).

---

### EGO-CRIT-08: Race Condition in Parallel Ego-Net Detection — Shared Write to personaOf Map

**What goes wrong:**
The natural parallelisation of Algorithm 1 is: for each node `v`, launch a
goroutine that (a) builds the ego-net, (b) runs inner `Detect`, and (c)
writes the community assignment into a shared `personaCommunityOf[v]` map.
Step (c) is a write to a shared map. In Go, concurrent map writes are a data
race and will be detected by `go test -race` and can cause a fatal crash in
production.

This is easy to introduce because the map write feels "safe" — each goroutine
writes a different key `v`. Go maps are not safe for concurrent writes even
when keys do not conflict.

**How to avoid:**
Use one of:
1. Pre-allocate a `[]egoResult` slice indexed by node position (not NodeID),
   write each goroutine's result to its own slice position (no sharing),
   then collect after all goroutines finish.
2. Use a `sync.Mutex` protecting the shared map (simple but serialises writes).
3. Use a channel to send results back to a single collector goroutine.

The slice approach is idiomatic Go and avoids any synchronisation on the hot path:

```go
nodes := g.Nodes()
results := make([]egoNetResult, len(nodes))

var wg sync.WaitGroup
sem := make(chan struct{}, runtime.NumCPU()) // bound concurrency

for i, v := range nodes {
    wg.Add(1)
    go func(idx int, node NodeID) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        results[idx] = detectEgoNet(g, node, innerDetector)
    }(i, v)
}
wg.Wait()
```

**Warning signs:**
- `go test -race` reports concurrent map write during `TestEgoSplittingDetect`.
- Intermittent panics in `ego_splitting.go` with "concurrent map write" message.

**Phase to address:** Phase implementing parallel ego-net detection; `go test -race` is the gate.

---

### EGO-CRIT-09: Ego-Net for Directed Graphs — Using Undirected Neighbor Set

**What goes wrong:**
The existing `CommunityDetector` implementations return
`ErrDirectedNotSupported`. The Ego Splitting Framework paper defines the
ego-net for undirected graphs. If the `EgoSplitting` implementation is later
extended to directed graphs, the definition of "neighbor" matters:
- **Out-neighbors only:** ego-net misses nodes that point *to* `v`.
- **In-neighbors only:** ego-net misses nodes that `v` points to.
- **Union of in + out:** the standard choice for directed ego-nets; corresponds
  to treating the underlying undirected graph.
- **Intersection:** ego-net contains only mutual-link neighbors — much smaller,
  often disconnected.

For the v1.2 milestone, the graph is undirected (existing constraint). This
pitfall is flagged to prevent a future directed-graph extension from silently
using the wrong neighbor set.

**How to avoid:**
Add an explicit guard in `EgoSplitting.Detect` matching the existing pattern:
```go
if g.IsDirected() {
    return OverlappingCommunityResult{}, ErrDirectedNotSupported
}
```
Document the limitation. When directed support is eventually added, the
ego-net construction must explicitly decide on the neighbor definition and
test it.

**Warning signs:**
- No guard for directed graphs in `EgoSplitting.Detect`.
- Directed graph test silently returns wrong results instead of an error.

**Phase to address:** Phase defining `OverlappingCommunityDetector` interface and stub.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Reuse `personaCommunityID = originalNodeID` for first persona | Simpler persona graph construction | Silent ID collisions on sparse-ID graphs; breaks `personaToOriginal` mapping | Never — always use independent counter |
| Skip ego-net deduplication and allow double-counted persona edges | Faster construction | Persona graph has 2x edge weight; modularity is wrong | Never |
| Use standard `nmi()` for overlapping accuracy tests | Reuses existing test infrastructure | Silently validates non-overlapping quality; misses the overlap correctness | Only as a supplementary check, never as the primary overlapping accuracy gate |
| One goroutine per ego-net without concurrency bound | Maximum parallelism | Goroutine explosion for large graphs (10K nodes = 10K goroutines); scheduler thrashing | Never for production code; acceptable only in single-threaded test paths |
| Include ego node `v` in its own ego-net | Simpler `Subgraph` call | Community detection finds `v` as hub; all neighbors in one community; no splits | Never |
| Assign singleton partition when ego-net has 0 edges (paper-accurate) | Correct per paper | Persona explosion for star-topology graphs | Acceptable only with `MaxPersonasPerNode` safety cap |

---

## Integration Gotchas

Specific to integrating Ego Splitting with the existing `package graph` codebase.

| Integration Point | Common Mistake | Correct Approach |
|-------------------|----------------|------------------|
| `Graph.Subgraph()` for ego-net | Pass `append(neighbors, v)` — includes ego node | Pass only `neighbors` (NodeIDs of `v`'s neighbors, excluding `v`) |
| `CommunityDetector` as inner detector | Instantiate one `louvainDetector` globally and call `Detect` concurrently | Safe — `louvainDetector` is immutable; `sync.Pool` inside is goroutine-safe |
| `CommunityResult.Partition` from inner detect | Use partition key as `NodeID` in persona graph directly | Partition keys are ego-net NodeIDs, which are *original* graph NodeIDs — keep them as original IDs for the mapping; create separate persona IDs |
| `LouvainOptions.InitialPartition` for inner ego-nets | Pass prior full-graph partition as warm start to each ego-net | Ego-net NodeIDs are a subset of original graph NodeIDs; the partition keys that exist in the ego-net are valid warm-start entries; those that don't exist are ignored by `reset()` — safe but verify no stale keys corrupt results |
| `ErrDirectedNotSupported` from inner detector | Not propagating error from inner `Detect` call | Always check and wrap inner `Detect` errors; inner calls on ego-net subgraphs can fail |
| `Graph.TotalWeight()` on persona graph | Assuming persona graph weight equals original graph weight | Persona graph can have different total weight (some edges may not find matching personas if ego-net is disconnected) |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Building ego-net via `Subgraph()` allocates new `Graph` per node | Memory spikes for dense graphs; GC pressure visible in `-benchmem` | Consider an in-place ego-net view struct that wraps the original graph with a node filter; avoid allocation per node | Graphs with average degree > 50 and N > 1K nodes |
| Unbounded goroutine creation in parallel ego-net phase | Scheduler thrashing; memory spike from goroutine stacks | Use a semaphore (`chan struct{}`) bounded to `runtime.NumCPU()` | N > 500 nodes with high-degree neighborhoods |
| `make(map[NodeID]struct{})` deduplication set in persona edge construction | O(N * avgDegree) map allocations, each GC'd | Pre-allocate one reusable `map[NodeID]struct{}` per goroutine; `clear()` between nodes | High-degree graphs (avgDegree > 100) |
| Inner detector `sync.Pool` thrashing with many small ego-nets | Pool hit rate near zero; allocs per op climbs | For ego-nets < 20 nodes, skip pool and allocate directly (`newLouvainState`) | N > 1K nodes where most ego-nets are tiny (sparse graphs) |
| `personaToOriginal` map growing to size of total personas | Acceptable but large; O(totalPersonas) memory | Profile memory at 10K nodes; persona count is bounded by 2 * |E| in worst case | Very dense graphs (|E| >> N) |

---

## "Looks Done But Isn't" Checklist

- [ ] **Ego-net excludes ego node:** Verify `buildEgoNet(g, v)` does NOT contain `v`; assert `egoNet.Nodes()` has no entry equal to `v`.
- [ ] **Persona IDs are disjoint from original IDs:** Assert `max(personaToOriginal.Keys()) >= g.NodeCount()` when original IDs are 0-indexed, OR assert the persona ID counter starts at 0 independently and never equals any original NodeID.
- [ ] **Overlapping result has actual overlaps:** Assert that for Karate Club, at least one node has `len(communities[node]) > 1` — if every node has exactly one community, the overlap detection silently degraded to non-overlapping.
- [ ] **Overlapping NMI is computed with `overlapNMI`, not `nmi`:** Grep test files for calls to `nmi(` in overlapping community tests — any such call is a bug.
- [ ] **Persona graph total weight matches original:** For a correct construction, `personaGraph.TotalWeight()` should equal `g.TotalWeight()` (each original edge maps to exactly one persona-graph edge).
- [ ] **`go test -race` passes on parallel ego-net detection:** The parallelisation of Algorithm 1 must pass the race detector; a sequential implementation that passes race but fails performance target is not "done".
- [ ] **Inner detector errors are propagated:** `EgoSplitting.Detect` must return an error if any inner `Detect` call fails, not silently skip the node.
- [ ] **Directed graph returns error:** `EgoSplitting.Detect` on a directed graph must return `ErrDirectedNotSupported`, matching the existing convention.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| EGO-CRIT-01: ego node included | LOW | Fix `buildEgoNet`; all tests re-run automatically |
| EGO-CRIT-02: persona ID collision | HIGH | Requires redesigning persona ID allocation; `personaToOriginal` map rebuild; all downstream tests must be revalidated |
| EGO-CRIT-03: double-counted edges | MEDIUM | Add deduplication set to persona edge construction; revalidate `TotalWeight` assertion; re-run NMI tests |
| EGO-CRIT-05: wrong recovery in Algo 3 | MEDIUM | Fix `recoverOverlappingCommunities` to collect all persona memberships; re-run overlap NMI tests |
| EGO-CRIT-06: wrong NMI implementation | MEDIUM | Implement `overlapNMI`; existing NMI tests are unaffected; new overlapping tests will need threshold calibration |
| EGO-CRIT-07: pool thrashing | LOW | Add size-tier logic or fresh allocation below threshold; performance fix only |
| EGO-CRIT-08: race condition | MEDIUM | Switch from shared map to indexed slice or channel collector; re-run with `-race` |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| EGO-CRIT-01: ego node in ego-net | Algorithm 1: ego-net construction | Unit test: `egoNet := buildEgoNet(g, v); assert !contains(egoNet.Nodes(), v)` |
| EGO-CRIT-02: persona ID collision | Algorithm 2: persona graph construction | Assert `personaToOriginal` is bijective; assert persona IDs never overlap original IDs |
| EGO-CRIT-03: double-counted edges | Algorithm 2: persona graph construction | Assert `personaGraph.TotalWeight() == g.TotalWeight()` |
| EGO-CRIT-04: star-node persona explosion | Algorithm 1 + Algorithm 2 | Benchmark on a star graph with 1K nodes; verify persona graph size is bounded |
| EGO-CRIT-05: wrong community recovery | Algorithm 3: community recovery | Assert at least one node has multiple memberships on Karate Club |
| EGO-CRIT-06: wrong NMI implementation | Accuracy validation phase | `overlapNMI` must differ from `nmi` on any graph with genuine overlaps |
| EGO-CRIT-07: pool thrashing | Algorithm 1 parallelisation | `-benchmem` shows allocs/op bounded; not proportional to N |
| EGO-CRIT-08: parallel write race | Algorithm 1 parallelisation | `go test -race ./graph/...` must pass |
| EGO-CRIT-09: directed graph no guard | Interface/stub phase | `EgoSplitting.Detect(directedGraph)` returns `ErrDirectedNotSupported` |

---

## Sources

- Epasto, A., Lattanzi, S., Paes Leme, R. (2017). Efficient Overlapping Community Detection via Ego Splitting. KDD 2017. https://dl.acm.org/doi/10.1145/3097983.3098054 — Algorithm 1, 2, 3 definitions; ego-net definition (egoless); persona construction
- McDaid, A.F., Greene, D., Hurley, N. (2011). Normalized Mutual Information to Evaluate Overlapping Community Finding Algorithms. arXiv:1110.2515 — overlapping NMI (NMI_max) formulation
- Existing `package graph` codebase pitfalls — `PITFALLS.md` v1.0/v1.1 (CRIT-01 through CONC-04) for integration context
- Go `sync.Pool` documentation and `map` concurrent access semantics — Go specification: concurrent map writes are undefined behaviour
- `Graph.Subgraph()` implementation in `graph.go:151` — canonical-key deduplication pattern (reference for EGO-CRIT-03 fix)
- Known pitfalls from milestone context: `bestSuperPartition` deep-copy, `rand.New(src)` in `reset()`, `commStr` rebuild in warm-start path — these are solved; ego-splitting must not reintroduce analogous patterns

---

*Pitfalls research for: Ego Splitting Framework — v1.2 milestone*
*Researched: 2026-03-30*
