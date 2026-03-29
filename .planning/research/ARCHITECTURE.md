# Architecture Patterns: Community Detection in Go

**Domain:** High-performance graph community detection (Louvain + Leiden)
**Researched:** 2026-03-29
**Overall confidence:** HIGH (stdlib patterns), MEDIUM (algorithm-specific optimizations)

---

## Recommended Architecture

### Package Structure: Stay in `package graph`

**Decision: Do NOT create sub-packages for Louvain/Leiden.**

Rationale:
- The existing codebase is a single `package graph` with zero external deps. Splitting creates import ceremony (`graph/louvain`, `graph/leiden`) with no benefit at this scale.
- `dominikbraun/graph` (the canonical Go graph library) uses a single-package approach for the same reason.
- The `CommunityDetector` interface, both algorithm structs, and all helper types should live in `graph/`.
- Only `graph/testdata/` remains a separate package (already established pattern).

**File layout:**

```
graph/
  graph.go          — existing: Graph, NodeID, Edge
  modularity.go     — existing: ComputeModularity*
  registry.go       — existing: NodeRegistry
  detector.go       — NEW: CommunityDetector interface, Options, Result types
  louvain.go        — NEW: LouvainDetector struct + Detect method
  leiden.go         — NEW: LeidenDetector struct + Detect method
  louvain_state.go  — NEW: internal phase state (unexported)
  leiden_state.go   — NEW: internal phase state (unexported)
  testdata/
    karate.go       — existing
    fixtures.go     — NEW: larger benchmark graphs (LFR, synthetic)
```

---

## CommunityDetector Interface

```go
// detector.go

type Partition map[NodeID]int

type DetectOptions struct {
    Resolution  float64 // gamma parameter; 1.0 = standard modularity
    MaxPasses   int     // 0 = run until convergence
    Seed        int64   // 0 = non-deterministic
}

type CommunityDetector interface {
    Detect(g *Graph, opts DetectOptions) (Partition, error)
}
```

**Why `(Partition, error)` not just `Partition`:**
- Algorithms can legitimately fail (e.g., degenerate graph, infinite loop guard). Returning error is idiomatic Go and matches the existing `(value, bool)` two-value idiom in `NodeRegistry`.
- `Partition` is a type alias over `map[NodeID]int` — zero overhead, compatible with existing `ComputeModularity`.

---

## Internal Data Structures

### Phase 1 State: `louvainState`

The hot path in Louvain phase 1 is the delta-modularity gain calculation: for each node, sum edge weights to each neighboring community, then evaluate `ΔQ = [k_i,in / m] - [Σtot * k_i / (2m²)]`.

```go
// louvain_state.go (unexported)

type communityID = int

type louvainState struct {
    // community assignment: node → community
    // flat slice indexed by NodeID — O(1) access, cache-friendly vs map
    assignment []communityID

    // per-community total internal weight (Σ_in)
    internalWeight []float64

    // per-community total degree (Σ_tot = sum of all edge weights for nodes in community)
    totalDegree []float64

    // node degree cache: avoids recomputing g.Strength() in inner loop
    nodeDegree []float64

    // neighbor community accumulator: reused per-node scratch buffer
    // maps communityID → sum of weights from current node to that community
    // allocated once per Detect call, reset per node via a "generation" trick or explicit zero
    neighborWeightBuf []float64
    neighborCommunityBuf []communityID // which community IDs were touched (for zeroing)

    totalWeight float64 // 2m — cached once
    numNodes    int
}
```

**Key allocation decisions:**

1. **`assignment []communityID` not `map[NodeID]communityID`** — NodeIDs in the existing `Graph` are dense integers (auto-created on `AddEdge`). A flat slice gives O(1) indexed access and is cache-line friendly. For a 10K-node graph this is 80 KB (10K × int64) vs potentially 640+ KB for a map with load factor overhead.

2. **`neighborWeightBuf []float64` pre-sized to `numNodes`** — The per-node inner loop accumulates edge weights by community. Using a flat slice indexed by communityID (same size as `numNodes` since communities ≤ nodes) and a companion `neighborCommunityBuf` slice to track which indices were touched allows O(1) reset (zero only touched entries) rather than allocating a fresh `map` per node. This eliminates the single biggest allocation hotspot in naive Louvain.

3. **`nodeDegree []float64` cached once** — `g.Strength(node)` iterates adjacency; caching before phase 1 loops converts O(N²) adjacency traversals to O(N) lookups.

### Phase 2 State: Supergraph Compression

Phase 2 collapses each community to a supernode and builds a new `*Graph` where edge weight = sum of cross-community edges.

```go
// louvain_state.go

// buildSupergraph returns a new *Graph where each node represents one community.
// communityMap maps old communityID → new NodeID in supergraph.
func buildSupergraph(g *Graph, state *louvainState) (*Graph, []communityID) {
    // 1. Assign new NodeIDs to unique communities (compact, 0-based)
    //    Use state.assignment scan — O(N), produces communityMap []int
    // 2. Create supergraph := NewGraph(g.directed)
    // 3. Iterate all edges in g; for each edge (u,v,w):
    //    cu = state.assignment[u], cv = state.assignment[v]
    //    if cu != cv: supergraph.AddEdge(communityMap[cu], communityMap[cv], w)
    //    else:        accumulate as self-loop (internal weight, not added as edge)
    // 4. Reset state.assignment to identity mapping on supergraph node IDs
    return supergraph, communityMap
}
```

**Why reuse `*Graph` for the supergraph rather than a custom struct:**
- `*Graph` already supports weighted edges and the `Neighbors`/`Strength` API that phase 1 depends on. Creating a bespoke `superGraph` type would duplicate this logic.
- The supergraph at level L typically has far fewer nodes than level 0 (Karate: 34 → 2-4 communities after one pass). Memory cost is negligible.
- The hierarchical result (full dendrogram) can be reconstructed from the sequence of `communityMap` slices if needed by a future phase.

### Leiden Additional State: Refined Partition

Leiden adds a refinement phase between phase 1 and phase 2. It requires a second partition array:

```go
type leidenState struct {
    louvainState                   // embed base state
    refinedAssignment []communityID // partition after refinement (used for supergraph)
    // queue of nodes whose neighborhood changed — Leiden visits only these after first pass
    dirtyQueue []NodeID
    dirtySet   []bool // O(1) membership test; same length as numNodes
}
```

The `dirtyQueue`/`dirtySet` pair is the key Leiden efficiency improvement over Louvain: after the initial full-graph pass, only enqueue nodes whose community assignment changed. This cuts repeated full-scan work on large sparse graphs.

---

## Concurrency Model

**Decision: Caller-owned goroutines — no internal worker pool.**

The use case is "many small graphs in parallel" (real-time GraphRAG queries). The caller already has concurrency control (HTTP handler goroutines, a pipeline stage, etc.). The right model is:

```go
// Detect is safe to call concurrently on different *Graph instances.
// Each call allocates its own louvainState; no shared mutable state.
func (l *LouvainDetector) Detect(g *Graph, opts DetectOptions) (Partition, error) { ... }
```

**Rules:**
1. **`Detect` is stateless on the receiver** — `LouvainDetector` holds only immutable config (default options). All mutable phase state lives in a `louvainState` allocated per `Detect` call.
2. **`*Graph` is read-only during `Detect`** — the algorithm reads edges via `g.Neighbors`/`g.Strength` but never mutates `g`. Document this contract: concurrent `Detect` calls on the same `*Graph` are safe IFF the caller does not mutate the graph concurrently.
3. **No internal goroutines spawned by `Detect`** — parallelizing within a single Detect call adds synchronization overhead that dominates for small graphs (< 50K nodes). The caller parallelizes across graphs.
4. **`sync.Pool` for `louvainState`** — allocating/zeroing the state slices (5× O(N) slices) costs ~microseconds for 10K nodes. A `sync.Pool` amortizes this across repeated calls:

```go
var louvainStatePool = sync.Pool{
    New: func() any { return &louvainState{} },
}

func (l *LouvainDetector) Detect(g *Graph, opts DetectOptions) (Partition, error) {
    st := louvainStatePool.Get().(*louvainState)
    st.init(g)           // resize slices if needed, zero contents
    defer func() {
        st.reset()       // clear slice contents but keep backing arrays
        louvainStatePool.Put(st)
    }()
    // ...
}
```

`st.init` uses `st.assignment = st.assignment[:numNodes]` (reslice if capacity sufficient, reallocate only if graph grew). This avoids allocation on the hot path for same-size or smaller graphs.

**What NOT to do:**
- Do not use a channel-based worker pool inside `Detect` — the overhead is ~1µs per goroutine handoff, which is significant for 10K-node graphs targeting < 100ms total.
- Do not hold any lock on `*Graph` inside `Detect` — read-only access is inherently safe for concurrent reads.
- Do not store phase state in `LouvainDetector` fields — makes the struct non-reentrant.

---

## Allocation Optimization Summary

| Hotspot | Naive Approach | Optimized Approach | Savings |
|---------|---------------|-------------------|---------|
| Per-node neighbor accumulator | `make(map[communityID]float64)` per node | Pre-sized `[]float64` + dirty list, reuse per node | Eliminates N map allocs per pass |
| Phase state slices | Allocate in each `Detect` call | `sync.Pool` of `*louvainState`; reslice if capacity ok | Eliminates 5 allocs per call |
| Node degree cache | Call `g.Strength(n)` in inner loop | Cache `[]float64` once before phase 1 | O(N) vs O(N²) adjacency traversals |
| Supergraph edges | `map[NodeID]map[NodeID]float64` | Iterate edges once, use `AddEdge` with accumulation | One pass, no intermediate map |
| Partition result | New `map[NodeID]int` per pass | Reuse `[]int` internally, convert to `map` once at return | N allocs → 1 alloc per final result |

---

## Scalability Considerations

| Concern | 1K nodes | 10K nodes | 100K nodes |
|---------|----------|-----------|------------|
| Phase 1 pass time | < 1ms | < 50ms (target) | ~500ms (acceptable) |
| State memory | ~400 KB | ~4 MB | ~40 MB |
| Supergraph nodes after 1 pass | ~50 | ~500 | ~5K |
| Concurrent calls | goroutine-safe by design | goroutine-safe | goroutine-safe |
| sync.Pool benefit | Low (small allocs) | Medium | High (large slice reuse) |

For 100K+ node graphs, consider a CSR (Compressed Sparse Row) adjacency representation instead of `map[NodeID][]Edge`. However, this requires changing the core `Graph` type — out of scope for this milestone. The current `map`-based adjacency is acceptable for the 10K-node target.

---

## Anti-Patterns to Avoid

### Anti-Pattern 1: Global Partition Lock
**What:** Using a `sync.Mutex` on a shared partition map across goroutines
**Why bad:** False sharing; contention for every node move destroys parallelism
**Instead:** Each `Detect` call owns its `louvainState` — no sharing, no locks

### Anti-Pattern 2: Recursive Supergraph Allocation
**What:** Rebuilding the full `louvainState` from scratch for each hierarchical level
**Instead:** `st.reinit(supergraph)` — resize slices in-place, avoid reallocation if capacity allows

### Anti-Pattern 3: Map for Neighbor Accumulation
**What:** `neighborWeights := make(map[int]float64)` inside the node-move inner loop
**Why bad:** For a hub node with 1K neighbors, this allocates a 1K-entry map per node per pass — O(N × passes) total allocations
**Instead:** Pre-sized `[]float64` with a dirty-index list as described above

### Anti-Pattern 4: Sub-package Split Too Early
**What:** `graph/louvain` and `graph/leiden` as separate packages
**Why bad:** Requires exporting `louvainState`, `supergraph helpers`, etc., bloating the public API surface
**Instead:** Stay in `package graph`; keep state types unexported

---

## Sources

- [Louvain method — Wikipedia](https://en.wikipedia.org/wiki/Louvain_method) — algorithm phase definitions
- [From Louvain to Leiden (Traag et al.)](https://www.nature.com/articles/s41598-019-41695-z) — Leiden refinement phase, dirty queue optimization (HIGH confidence)
- [GVE-Louvain: Fast Louvain in Shared Memory](https://arxiv.org/abs/2312.04876) — multicore implementation patterns, neighbor accumulator structure (MEDIUM confidence — abstract only)
- [Go sync.Pool mechanics — VictoriaMetrics](https://victoriametrics.com/blog/go-sync-pool/) — Pool reuse patterns (HIGH confidence)
- [Go Performance Optimization 2026](https://reintech.io/blog/go-performance-optimization-guide-2026) — worker pool vs per-call goroutine tradeoffs (MEDIUM confidence)
- [dominikbraun/graph — pkg.go.dev](https://pkg.go.dev/github.com/dominikbraun/graph) — single-package Go graph library precedent (HIGH confidence)
- [Organizing a Go module — go.dev](https://go.dev/doc/modules/layout) — official package layout guidance (HIGH confidence)

---

*Architecture research: 2026-03-29*
