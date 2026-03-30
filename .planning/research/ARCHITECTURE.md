# Architecture Patterns: Ego Splitting Framework Integration

**Domain:** Overlapping community detection via ego-splitting (Google, 2017) integrated into existing Go `package graph`
**Researched:** 2026-03-30
**Confidence:** HIGH (existing codebase, algorithmic paper) | MEDIUM (persona graph data structure trade-offs)

---

## Context: What Already Exists

The v1.0/v1.1 architecture is already implemented and must not change in any breaking way:

```
graph/
  graph.go          — Graph, NodeID, Edge (adjacency list, weighted, dir/undir)
  modularity.go     — ComputeModularity, ComputeModularityWeighted
  registry.go       — NodeRegistry (string <-> NodeID)
  detector.go       — CommunityDetector interface, CommunityResult, LouvainOptions, LeidenOptions
  louvain.go        — louvainDetector.Detect, phase1, buildSupergraph, normalizePartition
  louvain_state.go  — louvainState, sync.Pool acquire/release, reset (warm-start support)
  leiden.go         — leidenDetector.Detect, refinePartition
  leiden_state.go   — leidenState, sync.Pool acquire/release, reset (warm-start support)
  testdata/
    karate.go       — 34-node fixture
    football.go     — 115-node fixture
    polbooks.go     — 105-node fixture
```

Key invariants to preserve:
- `CommunityDetector` interface signature: `Detect(g *Graph) (CommunityResult, error)` — **unchanged**
- `LouvainOptions` / `LeidenOptions` — **unchanged**
- `louvainState` / `leidenState` pool pattern — **unchanged** (ego splitter creates its own internal instances)
- All existing tests pass without modification

---

## New Architecture: Ego Splitting Layer

### System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Caller (library user)                            │
│  detector := NewEgoSplitting(NewLouvain(opts))                       │
│  result, err := detector.Detect(g)  // OverlappingCommunityResult   │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ OverlappingCommunityDetector.Detect(*Graph)
┌──────────────────────────▼──────────────────────────────────────────┐
│                  egoSplittingDetector  (ego_splitting.go)            │
│                                                                      │
│  Algorithm 1: buildEgoNets(g)                                        │
│    └─> for each node v: extract ego-net subgraph                    │
│    └─> inner.Detect(egoNet) -> local partition per v                │
│                                                                      │
│  Algorithm 2: buildPersonaGraph(g, egoPartitions)                   │
│    └─> persona graph: *Graph with synthetic persona NodeIDs          │
│    └─> personaMap: PersonaID -> (originalNodeID, localCommID)        │
│                                                                      │
│  Algorithm 3: inner.Detect(personaGraph) -> persona partition        │
│    └─> mapPersonasToOriginal(personaPartition, personaMap)           │
│    └─> produce OverlappingCommunityResult                            │
└──────────────────────────┬──────────────────────────────────────────┘
                           │ CommunityDetector.Detect(*Graph)
                           │ (called N+1 times total: N ego-nets + 1 persona graph)
┌──────────────────────────▼──────────────────────────────────────────┐
│              Existing CommunityDetector (Louvain / Leiden)           │
│              louvainDetector.Detect  /  leidenDetector.Detect        │
│              — unchanged, no modification needed —                   │
└─────────────────────────────────────────────────────────────────────┘
```

### New Files

| File | Contents | Status |
|------|----------|--------|
| `graph/overlapping_detector.go` | `OverlappingCommunityDetector` interface, `OverlappingCommunityResult`, `EgoSplittingOptions` | **NEW** |
| `graph/ego_splitting.go` | `egoSplittingDetector` struct, `Detect` method (Algorithms 1-3) | **NEW** |
| `graph/persona.go` | `personaMap` type, `buildPersonaGraph`, `mapPersonasToOriginal` | **NEW** |
| `graph/ego_splitting_test.go` | Unit + integration tests, NMI validation | **NEW** |

### Modified Files

| File | Change | Risk |
|------|--------|------|
| `graph/detector.go` | None — only additive files needed | Zero |
| `graph/graph.go` | None | Zero |
| `graph/testdata/` | Possibly add ground-truth overlapping communities for NMI | LOW (additive only) |

---

## Interface and Type Definitions

### `overlapping_detector.go`

```go
package graph

// OverlappingCommunityDetector detects communities where a single node
// may belong to multiple communities simultaneously.
// Implementations must be safe for concurrent use on distinct *Graph instances.
type OverlappingCommunityDetector interface {
    Detect(g *Graph) (OverlappingCommunityResult, error)
}

// OverlappingCommunityResult holds the output of an overlapping detection run.
type OverlappingCommunityResult struct {
    // Communities is the primary output: each element is the set of NodeIDs
    // that belong to community i.  A NodeID may appear in multiple communities.
    Communities [][]NodeID

    // NodeCommunities is the inverse index: node -> sorted list of community IDs.
    // Derived from Communities; provided for O(1) per-node lookup.
    NodeCommunities map[NodeID][]int
}

// EgoSplittingOptions configures the EgoSplitting detector.
type EgoSplittingOptions struct {
    // Inner is the CommunityDetector used for both ego-net detection (Algorithm 1)
    // and persona-graph detection (Algorithm 3).  Required; no default.
    Inner CommunityDetector

    // MinEgoNetSize is the minimum number of neighbors a node must have for
    // its ego-net to be processed.  Nodes below this threshold are assigned
    // as a singleton community.  Default (0) treated as 1.
    MinEgoNetSize int
}

// NewEgoSplitting returns an OverlappingCommunityDetector using the
// Ego Splitting Framework (Epasto et al., Google 2017).
func NewEgoSplitting(opts EgoSplittingOptions) OverlappingCommunityDetector {
    return &egoSplittingDetector{opts: opts}
}
```

**Design rationale for `OverlappingCommunityResult`:**
- `Communities [][]NodeID` is the canonical representation in the paper and matches how NMI is computed against ground-truth sets. Callers iterating by community (the common GraphRAG pattern) get O(1) per community.
- `NodeCommunities map[NodeID][]int` is derived at construction time and enables O(1) "what communities does node X belong to?" — required for the persona-to-original mapping step and useful for callers.
- Deliberately NOT `map[NodeID][]int` as the sole field: the inverse form is harder to enumerate communities from, and ground-truth NMI requires community-set representation.

---

## Persona Graph Data Structure

### Decision: Reuse `*Graph` with Synthetic PersonaIDs

The persona graph is a standard weighted undirected graph — the same shape as the original graph but with nodes replaced by persona copies. Reusing `*Graph` is correct because:

1. `phase1` / `buildSupergraph` / `refinePartition` all operate on `*Graph` — the inner `CommunityDetector.Detect(*Graph)` call on the persona graph requires exactly a `*Graph`.
2. No new graph type is needed. The persona graph uses `NodeID` values in a new numeric range that does not overlap with original NodeIDs.
3. `Graph.Subgraph` already exists and is used for ego-net extraction.

### PersonaID Allocation

```
Original node v has NodeID = v (e.g., 0..N-1).
Persona nodes start at NodeID(N) and are allocated contiguously.

personaBase = NodeID(len(g.Nodes()))   // first persona NodeID
```

Each original node v gets one persona per local community detected in its ego-net. If v's ego-net yields k communities, v gets k persona nodes.

### `personaMap` — the Bridge Between Layers

```go
// persona.go

// personaEntry records what original node and local community a persona represents.
type personaEntry struct {
    Original  NodeID // original node in g
    LocalComm int    // community ID within v's ego-net partition
}

// personaMap maps PersonaID -> personaEntry.
// The inverse (original node -> []PersonaID) is also stored for edge wiring.
type personaMap struct {
    // forward: personaID -> (original, localComm)
    entries []personaEntry // indexed by (personaID - base)

    // inverse: originalNodeID -> list of personaIDs assigned to that node
    // Used in buildPersonaGraph to wire persona-graph edges.
    byOriginal map[NodeID][]NodeID

    base NodeID // first persona NodeID (= original node count)
}
```

This structure is an internal implementation detail inside `persona.go`. It is not exported. The persona graph and the personaMap are built together in `buildPersonaGraph` and consumed immediately in the same `Detect` call — no lifetime issue.

---

## Algorithm Implementation Map

### Algorithm 1: Ego-Net Construction and Local Detection

```
func buildEgoNets(g *Graph, inner CommunityDetector, minSize int) map[NodeID]CommunityResult

For each node v in g:
  1. neighbors := g.Neighbors(v)   — O(deg(v))
  2. if len(neighbors) < minSize: egoPartitions[v] = singleton; continue
  3. egoNet := g.Subgraph(neighborIDs(neighbors))  — reuses existing Subgraph method
     NOTE: ego-net is neighbors-only (excludes v itself), matching Algorithm 1 in paper
  4. result, err := inner.Detect(egoNet)
  5. egoPartitions[v] = result
```

Key detail: The paper's Algorithm 1 applies community detection to the *neighborhood* of v (ego-net, excluding v itself). The resulting partition determines how many personas v gets — one per community found in its neighborhood.

### Algorithm 2: Persona Graph Construction

```
func buildPersonaGraph(g *Graph, egoPartitions map[NodeID]CommunityResult,
                        minSize int) (*Graph, *personaMap)

1. Allocate personaMap with base = NodeID(g.NodeCount())
2. For each node v:
   a. If ego-net had 0 or 1 communities: assign single persona PersonaID(v)
      (degenerate: persona = original, no splitting needed)
   b. Else: assign one PersonaID per distinct community in egoPartitions[v]
   c. Record in personaMap.byOriginal[v]

3. Build persona graph (undirected, same weights as g):
   For each edge (u, v, w) in g:
     For each pair of personas (pu, pv) where:
       pu ∈ personaMap.byOriginal[u]  AND
       pv ∈ personaMap.byOriginal[v]  AND
       u and v are in the SAME local community of EACH OTHER's ego-net:
         personaGraph.AddEdge(pu, pv, w)
```

The edge-wiring condition is the core of the paper: edge (u,v) goes to persona pair (pu, pv) only when u and v co-appear in the same local community in both u's ego-net and v's ego-net. This implements the "consistent assignment" principle.

**Implementation note:** To check co-membership efficiently, store a lookup:

```go
// For ego-net of u: commOfVInU[v] = community ID of v in u's ego-net partition
// (only defined for v in u's neighborhood)
commInEgoNet := make(map[NodeID]map[NodeID]int, g.NodeCount())
// commInEgoNet[u][v] = local comm of v in u's ego-net
```

This is O(sum of ego-net sizes) = O(N * avg_degree) in space, which is the same as the graph itself.

### Algorithm 3: Persona Graph Detection and Result Recovery

```
func (d *egoSplittingDetector) Detect(g *Graph) (OverlappingCommunityResult, error)

1. egoPartitions := buildEgoNets(g, d.opts.Inner, minSize)
2. personaGraph, pm := buildPersonaGraph(g, egoPartitions, minSize)
3. personaResult, err := d.opts.Inner.Detect(personaGraph)
4. result := mapPersonasToOriginal(personaResult.Partition, pm)
5. return result, nil
```

`mapPersonasToOriginal` groups persona nodes by their global community, then for each global community collects the set of original nodes whose personas appear in it. Each such set is one overlapping community.

---

## Data Flow: Complete Call Path

```
Detect(g *Graph)
    │
    ├─ buildEgoNets(g, inner, minSize)
    │    ├─ for v in g.Nodes():
    │    │    egoNet = g.Subgraph(neighbors(v))   [*Graph reused]
    │    │    inner.Detect(egoNet)                [CommunityDetector, existing]
    │    │    → egoPartitions[v] = CommunityResult
    │    └─ return map[NodeID]CommunityResult
    │
    ├─ buildPersonaGraph(g, egoPartitions, minSize)
    │    ├─ allocate personaMap (personaID → original, localComm)
    │    ├─ NewGraph(false)                       [*Graph reused]
    │    ├─ wire edges via co-membership check
    │    └─ return (*Graph, *personaMap)
    │
    ├─ inner.Detect(personaGraph)                 [CommunityDetector, existing]
    │    → personaResult CommunityResult
    │
    └─ mapPersonasToOriginal(personaResult.Partition, pm)
         ├─ group personas by global community ID
         ├─ for each group: collect original NodeIDs (dedup)
         ├─ build Communities [][]NodeID
         ├─ build NodeCommunities map[NodeID][]int
         └─ return OverlappingCommunityResult
```

---

## File-by-File Integration Points

### `overlapping_detector.go` — New, zero deps on other new files

Exports: `OverlappingCommunityDetector`, `OverlappingCommunityResult`, `EgoSplittingOptions`, `NewEgoSplitting`
Imports (within package): `CommunityDetector`, `NodeID` (from `graph.go`, `detector.go`)
No breaking changes. Purely additive.

### `persona.go` — New, internal only

Exports: nothing (all unexported)
Types: `personaEntry`, `personaMap`
Functions: `buildPersonaGraph`, `mapPersonasToOriginal`
Imports: `graph.go` (`Graph`, `NodeID`, `Edge`), `detector.go` (`CommunityResult`)
Used by: `ego_splitting.go` only

### `ego_splitting.go` — New, orchestrates the algorithm

Exports: nothing (struct is unexported; constructor `NewEgoSplitting` is in `overlapping_detector.go`)
Types: `egoSplittingDetector`
Functions: `buildEgoNets`, `(d *egoSplittingDetector).Detect`
Imports: all within `package graph`

### `ego_splitting_test.go` — New

Tests: unit tests per algorithm step, integration tests on Karate/Football/Polbooks, NMI validation, benchmark, race test

---

## Concurrency Model

The ego-splitting Detect method follows the same pattern as Louvain/Leiden:

- `egoSplittingDetector` is stateless — only `EgoSplittingOptions` (immutable config) is on the struct.
- All mutable state is stack-local or heap-allocated per `Detect` call.
- The `inner.Detect` calls (on ego-nets) are sequential per the paper's algorithm. Opportunistic parallelism across ego-nets is possible in a future optimization but is out of scope for v1.2.
- `go test -race` must pass: no goroutines are spawned inside `Detect`.

**Why no ego-net parallelism in v1.2:**
Ego-nets for a 10K-node graph with average degree 20 produce ~10K small subgraph Detect calls averaging ~20 nodes each. Goroutine overhead (~2µs per goroutine) would dominate these sub-microsecond Detect calls. Parallelism only pays off when inner Detect is slow (>1ms), which happens for larger ego-nets. The 200-300ms total budget is achievable sequentially.

**sync.Pool interaction:**
The inner `CommunityDetector` reuses `louvainStatePool` / `leidenStatePool`. These pools are goroutine-safe and work correctly with the sequential ego-net loop — each inner `Detect` call acquires and releases from the pool independently. No changes needed to pool logic.

---

## Recommended Build Order

Phases should be ordered by dependency, with each phase leaving the package in a passing-tests state.

```
Phase 1: Types and Interfaces
  - overlapping_detector.go: OverlappingCommunityDetector, OverlappingCommunityResult, EgoSplittingOptions, NewEgoSplitting stub
  - Confirms: package compiles, no breaks to existing tests

Phase 2: Persona Graph
  - persona.go: personaEntry, personaMap, buildPersonaGraph, mapPersonasToOriginal
  - Unit tests for persona construction on small hand-crafted graphs
  - Confirms: persona node allocation correct, edge wiring correct

Phase 3: Ego-Net Construction (Algorithm 1)
  - ego_splitting.go: buildEgoNets function
  - Unit tests: ego-net extraction returns correct subgraph per node
  - Confirms: Subgraph reuse works, inner.Detect called correctly

Phase 4: Full Detect (Algorithms 2+3) + Integration
  - ego_splitting.go: egoSplittingDetector.Detect (full pipeline)
  - ego_splitting_test.go: Karate/Football/Polbooks integration tests
  - Confirms: end-to-end result is non-empty, NMI >= threshold

Phase 5: Accuracy Validation and Benchmarks
  - NMI measurement against ground-truth overlapping communities
  - Benchmark against 10K synthetic graph: target 200-300ms
  - go test -race pass

Phase 6: Edge Cases
  - Empty graph, single-node graph, disconnected graph, star graph
  - Nodes with no ego-net (degree 0), nodes where all neighbors form one community
```

**Dependency chain:**
- Phase 2 (persona) depends on Phase 1 (types) — personaMap references `CommunityResult`
- Phase 3 (ego-net) depends on Phase 1 — calls `inner.Detect` returning `CommunityResult`
- Phase 4 (full Detect) depends on Phases 2 and 3
- Phases 5-6 depend on Phase 4

---

## Scalability Considerations

| Concern | 1K nodes | 10K nodes | 100K nodes |
|---------|----------|-----------|------------|
| Ego-net Detect calls | ~1K calls × tiny graphs | ~10K calls × small graphs | ~100K calls — sequential bottleneck |
| Persona graph node count | up to ~3-5x original | up to ~3-5x original | up to ~3-5x original |
| Persona graph edge count | similar order to original | similar order to original | similar order |
| Total time budget | < 30ms | 200-300ms (target) | ~3-5s (out of scope) |
| Memory (persona graph) | small | ~3-5x graph memory | out of scope |

For 100K+ nodes, ego-net parallelism (goroutine pool over the N inner Detect calls) becomes attractive. This is a clean future optimization — the `buildEgoNets` signature is already separable enough to parallelize without changing the interface.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: New Graph Type for Persona Graph

**What people do:** Define a `PersonaGraph` struct to "make clear" what it holds.
**Why it's wrong:** The inner `CommunityDetector.Detect` requires `*Graph`. A new type forces conversion. No benefit — persona nodes are just NodeIDs in a different numeric range.
**Instead:** Use `*Graph` directly, document the NodeID range convention.

### Anti-Pattern 2: Storing Persona Mappings Inside Graph

**What people do:** Add a `personaOf map[NodeID]NodeID` field to `Graph` or encode it in node weights.
**Why it's wrong:** Pollutes the general-purpose `Graph` type with algorithm-specific state. Creates a maintenance burden for every future algorithm.
**Instead:** Keep `personaMap` purely in `persona.go`, scoped to the lifetime of one `Detect` call.

### Anti-Pattern 3: Merging OverlappingCommunityDetector into CommunityDetector

**What people do:** Add `DetectOverlapping` to the existing `CommunityDetector` interface, or change the return type.
**Why it's wrong:** Breaking change. All existing implementations (Louvain, Leiden) would need to add a no-op method. The two detection paradigms have different result shapes.
**Instead:** Separate `OverlappingCommunityDetector` interface. Both interfaces coexist in `detector.go` / `overlapping_detector.go`. The ego splitter satisfies `OverlappingCommunityDetector`; Louvain/Leiden satisfy `CommunityDetector`.

### Anti-Pattern 4: Using NodeID(0..N-1) Overlap for Personas

**What people do:** Re-use original NodeIDs for personas with some offset computed per-node, leading to collision risk.
**Why it's wrong:** If original graph has nodes 0..N-1, personas starting at any value < N collide. Off-by-one errors are silent.
**Instead:** `personaBase = NodeID(g.NodeCount())` — personas occupy `[N, N+P)` where P = total persona count. Simple, collision-free, easy to document.

### Anti-Pattern 5: Exporting `personaMap`

**What people do:** Export the persona-to-original mapping in `OverlappingCommunityResult` for "debugging".
**Why it's wrong:** Exposes internal algorithm structure as public API. Future algorithm changes (e.g. hierarchical ego splitting) would require breaking API changes.
**Instead:** `OverlappingCommunityResult` exposes only what callers need: `Communities [][]NodeID` and `NodeCommunities map[NodeID][]int`. The persona mapping is an implementation detail.

---

## Integration Points Summary

| Integration Point | What Connects | Contract |
|------------------|---------------|----------|
| `NewEgoSplitting(opts)` receives `CommunityDetector` | Caller wires inner algorithm | Any `CommunityDetector` (Louvain, Leiden, future) works |
| `inner.Detect(egoNet *Graph)` | ego_splitting.go → louvain.go / leiden.go | Read-only graph; returns `CommunityResult` |
| `inner.Detect(personaGraph *Graph)` | ego_splitting.go → louvain.go / leiden.go | Same contract; persona graph is a valid `*Graph` |
| `g.Subgraph(nodeIDs)` | ego_splitting.go → graph.go | Already implemented; ego-net extraction |
| `g.Neighbors(v)` | ego_splitting.go → graph.go | Read-only; used to enumerate ego-net members |
| `louvainStatePool` / `leidenStatePool` | unchanged; inner Detect calls acquire/release | No modification; pool is per-Detect-call scoped |

---

## Sources

- Epasto, A., Lattanzi, S., Paes Leme, R. (2017). *Ego-Splitting Framework: from Non-Overlapping to Overlapping Clustering.* KDD 2017. — Algorithm 1-3 definitions, persona graph construction (HIGH confidence — primary source)
- Existing codebase: `graph/graph.go`, `graph/detector.go`, `graph/louvain.go`, `graph/leiden.go` — interface contracts, Graph API (HIGH confidence — direct code inspection)
- `graph/louvain_state.go`, `graph/leiden_state.go` — sync.Pool pattern, reset/acquire/release lifecycle (HIGH confidence — direct code inspection)

---

*Architecture research for: Ego Splitting Framework integration into package graph*
*Researched: 2026-03-30*
