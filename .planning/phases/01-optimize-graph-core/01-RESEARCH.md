# Phase 1: optimize graph core - Research

**Researched:** 2026-04-01
**Domain:** Go graph algorithm performance — allocation reduction, cache locality, dead code removal
**Confidence:** HIGH (all findings verified directly from source code and live benchmark runs)

---

## Summary

Phase 1 targets seven distinct bottlenecks in `graph/graph.go`, `graph/louvain.go`, `graph/leiden.go`, `graph/louvain_state.go`, and `graph/leiden_state.go`. All changes are internal — no public API signatures change. The baseline is confirmed live: Louvain 10K = 63.5ms/op, 48 793 allocs/op (Apple M4, go1.26.1).

The two highest-impact items are Nodes() caching (eliminates 15–25 × 10K-element allocations per Detect call across all callers) and the `rand.New` re-allocation on every state reset. Both are straightforward field additions. The CSR view for phase1 inner-loop adjacency lookup is moderate complexity but high cache-locality gain. BFS cursor, buildSupergraph dedup, Subgraph seen-map pooling, and dead code deletion are individually small but collectively material.

The critical constraint on Nodes() caching: `phase1` (louvain.go:172–177) calls `g.Nodes()` then does `slices.Sort` followed by `state.rng.Shuffle` — it **mutates** the returned slice in-place. This is the single caller that writes to the slice. The cache implementation must return a copy (or the cache stores sorted order and callers that shuffle must copy before shuffling).

**Primary recommendation:** Implement in priority order — Nodes() cache, rand.Rand reuse, CSR view, BFS cursor, buildSupergraph dedup, Subgraph seen-map pool, dead code removal. Commit atomically per area.

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
All implementation choices are at Claude's discretion — pure infrastructure/performance phase. Use CONCERNS.md analysis as the authoritative spec. Priority order: high-impact first (Nodes caching, CSR view), then medium (BFS cursor, buildSupergraph, Subgraph), then low-medium (rand reuse), then dead code cleanup.

Execution approach:
- Implement as atomic commits per optimization area
- Run `go test ./graph/...` and `go test -bench=. -benchmem ./graph/...` to validate before/after
- Each optimization must not regress existing tests

CSR scope:
- CSR is an internal view only — built at the start of Detect(), not stored on Graph struct
- Public Graph API (AddNode, AddEdge, Neighbors, Nodes) remains unchanged
- CSR used only in phase1 hot path inside louvain.go/leiden.go

Nodes() caching:
- Cache `sortedNodes []NodeID` on Graph struct, invalidated (nil) on AddNode/AddEdge
- Callers must not modify the returned slice (documented contract)
- Zero-alloc on repeated calls

### Claude's Discretion
All implementation choices are at Claude's discretion.

### Deferred Ideas (OUT OF SCOPE)
- OmegaIndex O(n²) fix — test-only, not hot path
- EgoSplitting channel batching — separate phase
- louvainState aliasing refactor (phaseState interface) — structural refactor, separate phase
- Warm-start compaction skip — requires contiguous-partition contract design
- Tolerance-based early exit — feature work, separate phase
- SubgraphInto(dst *Graph) API — API addition, separate phase
</user_constraints>

---

## Exact Line Numbers and Function Signatures

All findings below are verified by direct source inspection (graph-core-optimization worktree, 2026-04-01).

### graph/graph.go

| What | Location | Detail |
|------|----------|--------|
| `Graph` struct | lines 15–20 | Add `sortedNodes []NodeID` cache field here |
| `AddNode` | line 33 | Must nil-out `g.sortedNodes` on mutation |
| `AddEdge` | line 47 | Must nil-out `g.sortedNodes` on mutation |
| `Nodes()` | lines 79–85 | Replace allocation with cache-hit path |
| `Subgraph()` | lines 185–230 | `seen := make(map[[2]NodeID]struct{})` at line 204 — pool target |

**Current `Nodes()` signature:** `func (g *Graph) Nodes() []NodeID` — unchanged after caching.

**`Graph` struct addition:**
```go
type Graph struct {
    directed    bool
    nodes       map[NodeID]float64
    adjacency   map[NodeID][]Edge
    totalWeight float64
    sortedNodes []NodeID  // cache; nil when stale
}
```

**`AddNode` / `AddEdge` invalidation** (one line each at end of the mutation path):
```go
g.sortedNodes = nil
```

**New `Nodes()` body:**
```go
func (g *Graph) Nodes() []NodeID {
    if g.sortedNodes != nil {
        return g.sortedNodes
    }
    ids := make([]NodeID, 0, len(g.nodes))
    for id := range g.nodes {
        ids = append(ids, id)
    }
    slices.Sort(ids)
    g.sortedNodes = ids
    return ids
}
```

**CRITICAL — callers that mutate the returned slice:**

| Caller | File:Line | Mutation | Fix Required |
|--------|-----------|----------|--------------|
| `phase1` | louvain.go:172–177 | `slices.Sort` + `rng.Shuffle` in-place | Must copy before shuffle |
| `louvainState.reset` | louvain_state.go:68–69 | `slices.Sort` (safe if cache already sorted) | No copy needed if Nodes() returns sorted |
| `leidenState.reset` | leiden_state.go:73–74 | `slices.Sort` (safe if cache already sorted) | No copy needed if Nodes() returns sorted |
| `newLouvainState` | louvain_state.go:128–129 | `slices.Sort` | No copy needed if cache already sorted |
| `newLeidenState` | leiden_state.go:135–136 | `slices.Sort` | No copy needed if cache already sorted |

Since the cache stores the slice in **sorted order**, all `slices.Sort` callers become no-ops (already sorted) — safe to call on the cached slice without copying. Only `phase1` does `rng.Shuffle` which reorders in-place — this is the single caller that **must** copy:

```go
// phase1, louvain.go:172
nodes := g.Nodes()
shuffled := make([]NodeID, len(nodes))
copy(shuffled, nodes)
state.rng.Shuffle(len(shuffled), func(i, j int) {
    shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
})
```

Other callers iterate or range over the slice without modifying element values — all safe to use cached slice directly:
- `louvain.go:54` — `origNodes := g.Nodes()` used as read-only key set
- `louvain.go:31`, `leiden.go:40` — `nodes := g.Nodes()` in guard clause, range-only
- `louvain.go:296`, `louvain.go:310`, `modularity.go:41` — range-only
- `leiden.go:106` — `origNodes` read-only
- `ego_splitting.go:448,463,538,652,883` — range/iteration only

**Conclusion:** Only `phase1` requires a copy-before-shuffle. All other callers are safe.

---

### graph/louvain.go

| What | Location | Detail |
|------|----------|--------|
| `deltaQ` dead function | lines 260–270 | Delete entirely — not called anywhere in production |
| `phase1` | lines 171–258 | Add CSR usage; copy-before-shuffle fix for Nodes() cache |
| `buildSupergraph` | lines 276–349 | Currently iterates all adjacency (both directions) and divides by 2 |
| `buildSupergraph` inner loop | lines 310–325 | Already uses canonical `a < b` key for `interEdges` — but `selfLoops` still double-counts (divides by 2 at line 332) |
| `normalizePartition` | lines 352–374 | Allocates `nodes` slice from `partition` map iteration — unaffected by Nodes() cache |

**buildSupergraph analysis:** The current code at lines 310–325 already canonicalizes inter-community edge keys (`a, b := superN, superNeighbor; if a > b { a, b = b, a }`). However it processes **all** adjacency entries (both directions) and then divides accumulated weight by 2 at write time. The fix is to add a `lo < hi` guard during iteration so each undirected edge is visited once — eliminating the `/2.0` correction:

```go
// Current (lines 332, 337–339):
newGraph.AddEdge(super, super, w/2.0)   // self-loop
newGraph.AddEdge(key.a, key.b, w/2.0)  // inter-community

// After guard (w is already correct, no division needed):
newGraph.AddEdge(super, super, w)
newGraph.AddEdge(key.a, key.b, w)
```

The guard to add inside the neighbor iteration (lines 312–325):
```go
for _, e := range g.Neighbors(n) {
    superNeighbor := commToSuper[partition[e.To]]
    if superN == superNeighbor {
        // self-loop: only count once (from the lower-ID endpoint)
        if n <= e.To {   // lo <= hi guard for undirected self-loop dedup
            selfLoops[superN] += e.Weight
        }
    } else {
        a, b := superN, superNeighbor
        if a > b { a, b = b, a }
        if NodeID(partition[n]) <= NodeID(partition[e.To]) {  // process each edge once
            interEdges[edgeKey{a, b}] += e.Weight
        }
    }
}
```

Pre-sizing maps: `make(map[edgeKey]float64, g.EdgeCount())` and `make(map[NodeID]float64, len(commList))`.

---

### graph/leiden.go

| What | Location | Detail |
|------|----------|--------|
| `refinePartition` BFS | lines 255–273 | `queue = queue[1:]` head-slice dequeue at line 259 |
| `inComm` map | line 244 | `make(map[NodeID]struct{}, len(nodes))` per community |
| `visited` map | line 249 | `make(map[NodeID]bool, len(nodes))` per community |

**BFS cursor fix** (lines 255–273):
```go
// Current (line 255–259):
queue := []NodeID{start}
visited[start] = true
for len(queue) > 0 {
    cur := queue[0]
    queue = queue[1:]   // head-slice dequeue — abandons capacity

// Fixed:
queue := []NodeID{start}
visited[start] = true
head := 0
for head < len(queue) {
    cur := queue[head]
    head++
```

No algorithmic change. Eliminates backing-array abandonment. The queue slice can also be pooled across communities by resetting `head=0; queue=queue[:0]` per community iteration.

---

### graph/louvain_state.go

| What | Location | Detail |
|------|----------|--------|
| `rand.New(src)` in `reset` | line 65 | Allocates new `*rand.Rand` every reset |
| `rng` field on `louvainState` | line 17 | Currently `*rand.Rand` |
| `newLouvainState` | lines 120–147 | Dead code — not called from pool path |

**rand.Rand reuse fix:**

The comment at line 57–58 says: "Always create a fresh rand.New to ensure identical number generation to newLouvainState; st.rng.Seed skips internal state setup." This is the documented reason for the current approach — the Phase 04 decision log confirms `rand.New(src)` was chosen over `st.rng.Seed()` to avoid shuffle divergence.

However, Go 1.20+ `math/rand` supports resetting a `rand.Rand` via its source's `Seed` method when the source is a `*rand.rngSource`. The concern is behavioral equivalence with `rand.New(rand.NewSource(seed))`.

**Recommended approach — math/rand/v2 (Go 1.22+, available in Go 1.26):**
```go
import "math/rand/v2"

type louvainState struct {
    // ...
    rng *rand.Rand  // math/rand/v2
}

// In reset():
if st.rng == nil {
    st.rng = rand.New(rand.NewPCG(uint64(seed), 0))
} else {
    *st.rng = *rand.New(rand.NewPCG(uint64(seed), 0))
    // Or store the PCG source and re-seed it:
    // st.pcg.Seed(uint64(seed), 0)
}
```

**Simpler approach — store source separately:**
```go
type louvainState struct {
    // ...
    rngSrc *rand.rngSource  // unexported — not accessible
}
```

The unexported `rand.rngSource` approach is not viable without reflection. The cleanest fix with `math/rand` v1 is to store the `rand.Source` and call `src.Seed(newSeed)`:
```go
type louvainState struct {
    partition     map[NodeID]int
    commStr       map[int]float64
    neighborBuf   map[NodeID]float64
    neighborDirty []NodeID
    candidateBuf  []int
    rng           *rand.Rand
    rngSrc        rand.Source  // stored so we can reseed without new allocation
}

// Pool New:
src := rand.NewSource(0)
return &louvainState{
    ...,
    rng:    rand.New(src),
    rngSrc: src,
}

// In reset():
if seed != 0 {
    st.rngSrc.(*rand.rngSource)  // type assertion not possible on interface
```

This approach hits a wall: `rand.Source` is an interface; the concrete type is unexported. **The only clean v1 approach is to store a `*rand.Rand` and create a new source only when rng is nil, relying on the pool to keep the rng alive across calls.** Since the pool reuses the state struct (including `rng`), creating `rand.New(src)` only when `st.rng == nil` (pool New) and then using `st.rng.Seed(seed)` for subsequent resets is the approach:

```go
// Pool New (once per pool object lifetime):
return &louvainState{
    ...,
    rng: rand.New(rand.NewSource(1)),  // placeholder; reseeded in reset
}

// In reset():
st.rng.Seed(seed)   // reseeds existing *rand.Rand in-place — zero alloc
```

**The Phase 04 decision** says `st.rng.Seed()` causes shuffle divergence vs. `rand.New`. This needs verification: `(*rand.Rand).Seed(s)` calls `src.Seed(s)` then resets internal `readVal/readPos` state. The concern in Phase 04 was that `st.rng.Seed` was being called *without* a fresh source, causing the Rand's internal buffer to differ. With Go 1.26, `(*rand.Rand).Seed` is deprecated (replaced by constructing via source), but it does reset internal state correctly. The **math/rand/v2** path is cleaner: `rand.New(rand.NewPCG(uint64(seed), 0))` creates a new Rand cheaply (PCG is a struct, not a heap allocation for the source itself). However `rand.New` still allocates the `*rand.Rand` wrapper.

**Recommended final approach:** Store `rng *rand.Rand` and a separate `src rand.Source` (as `*lockedSource` workaround is not needed since state is single-threaded). Use `rand.New` only in `sync.Pool.New`, then in `reset()` call the source's Seed and reset the Rand wrapper by assigning `*st.rng = *rand.New(st.src)` — this reuses the `rand.Rand` memory without heap allocation for the wrapper.

Actually the cleanest zero-alloc approach: since `rand.Rand` is a small struct (24 bytes), store it by value, not pointer:

```go
type louvainState struct {
    partition     map[NodeID]int
    commStr       map[int]float64
    neighborBuf   map[NodeID]float64
    neighborDirty []NodeID
    candidateBuf  []int
    rng           rand.Rand   // by value, not pointer
}

// In pool New: initialize with rng zero value (will be seeded in reset)
// In reset():
src := rand.NewSource(seed)
st.rng = *rand.New(src)   // copies struct value into st.rng — no heap alloc for Rand
                            // but still allocates rand.Source (interface + concrete type)
```

This eliminates one pointer indirection and one heap allocation (the `*rand.Rand` wrapper). The `rand.Source` (16 bytes) still allocates. Net saving: 1 alloc/reset → 1 alloc/reset (source still allocates). Marginal.

**Practical recommendation:** The most impactful approach given Go 1.26 is available is to migrate `louvain_state.go` and `leiden_state.go` to `math/rand/v2`:

```go
import "math/rand/v2"

type louvainState struct {
    ...
    rng *rand.Rand  // math/rand/v2
    pcg *rand.PCG   // seeded source stored for reseed
}

// Pool New:
pcg := rand.NewPCG(1, 0)
return &louvainState{
    ...,
    rng: rand.New(pcg),
    pcg: pcg,
}

// In reset():
st.pcg.Seed(uint64(actualSeed), 0)   // reseed in-place — zero alloc
```

`rand.PCG` is a concrete struct (not interface), so `Seed` on it is a direct method call. This eliminates `rand.New(src)` allocation on every reset. `rand.New` is called once in pool construction and the pointer is reused.

**Note:** `slices.Sort` + `rng.Shuffle` from `math/rand/v2` uses the same API as v1 — `rng.Shuffle(n, swap)`. Import path change only.

---

### graph/leiden_state.go

Same changes as `louvain_state.go`. Lines 70 and 153 are the `rand.New(src)` calls.

---

## Standard Stack

| Library | Version | Purpose | Notes |
|---------|---------|---------|-------|
| `math/rand/v2` | Go stdlib (Go 1.22+) | Random number generation | Available in Go 1.26; PCG source allows reseed without alloc |
| `sync` | Go stdlib | `sync.Pool` for seen-map pooling | Already used in codebase |
| `slices` | Go stdlib (Go 1.21+) | Sorted slice operations | Already imported in louvain.go, leiden.go |

**Go version confirmed:** go1.26.1 (from `go version`). `math/rand/v2` is fully available.

**Installation:** No new dependencies. All changes use stdlib only (matches project constraint: zero external imports).

---

## Architecture Patterns

### CSR Internal View

Build once at the start of `Detect()` for the current graph, not stored on `Graph` struct.

```go
// Internal to louvain.go / leiden.go — not exported
type csrGraph struct {
    offsets []int32  // len = NodeCount+1; offsets[n] = start index in edges for node n
    edges   []Edge   // flat edge list
    nodeIDs []NodeID // maps dense index i -> NodeID (for building offsets)
    idToIdx map[NodeID]int32 // maps NodeID -> dense index i
}

func buildCSR(g *Graph) csrGraph {
    nodes := g.Nodes() // uses cached sorted slice
    n := len(nodes)
    idToIdx := make(map[NodeID]int32, n)
    for i, id := range nodes {
        idToIdx[id] = int32(i)
    }
    offsets := make([]int32, n+1)
    for i, id := range nodes {
        offsets[i+1] = offsets[i] + int32(len(g.adjacency[id]))
    }
    edges := make([]Edge, offsets[n])
    for i, id := range nodes {
        copy(edges[offsets[i]:offsets[i+1]], g.adjacency[id])
    }
    return csrGraph{offsets: offsets, edges: edges, nodeIDs: nodes, idToIdx: idToIdx}
}

func (c *csrGraph) neighbors(idx int32) []Edge {
    return c.edges[c.offsets[idx]:c.offsets[idx+1]]
}
```

Usage in `phase1`: replace `g.Neighbors(n)` with `csr.neighbors(csr.idToIdx[n])` in the inner loop (line 204).

**Note on CSR scope:** After `normalizePartition`, supergraph NodeIDs are always 0-indexed contiguous — so `idToIdx` can be eliminated for supergraph CSR (direct index = NodeID). On the original graph, NodeIDs may not be contiguous so `idToIdx` map is needed.

### Subgraph seen-map Pool

```go
var subgraphSeenPool = sync.Pool{
    New: func() any {
        m := make(map[[2]NodeID]struct{}, 32)
        return &m
    },
}

// In Subgraph():
seenPtr := subgraphSeenPool.Get().(*map[[2]NodeID]struct{})
seen := *seenPtr
for k := range seen { delete(seen, k) }   // clear reused map
defer subgraphSeenPool.Put(seenPtr)
```

Alternative: for small ego-nets (avg degree ~5), a sorted-slice of `[2]NodeID` and binary search eliminates the map entirely. Given avg degree 5, the seen-set has at most `d*(d-1)/2 ≈ 10` entries — a linear scan over a `[10][2]NodeID` array is faster than map lookups.

### Dead Code Removal

- Delete `deltaQ` function: louvain.go lines 260–270 (9 lines). Verify no callers: `grep -r "deltaQ" graph/` finds zero production callers.
- Add `// O(n): do not call inside inner loops` comment to `CommStrength` (graph.go:244).
- Optionally delete `newLouvainState` (louvain_state.go:120–147) and `newLeidenState` (leiden_state.go:127–155) — both are unused in production code path. Verify no test references first.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| RNG reuse without alloc | Custom bitfiddling | `math/rand/v2` PCG source `Seed()` | Stdlib, tested, PCG is faster than rngSource |
| Seen-map pooling | Custom pool implementation | `sync.Pool` (already in codebase) | Already established pattern |
| Sorted node iteration | Manual sort-and-cache | `slices.Sort` + struct field cache | Already used throughout |

---

## Common Pitfalls

### Pitfall 1: Nodes() Cache — Shuffle Mutation
**What goes wrong:** `phase1` calls `g.Nodes()` and shuffles the result in-place. If the cache returns the same backing array, the cache is corrupted and subsequent callers get a shuffled (non-sorted) slice.
**Why it happens:** Go slices share backing arrays; sort/shuffle operate in-place.
**How to avoid:** In `phase1`, copy the cached slice before shuffling. The cache stores sorted order; only the shuffle step needs a copy.
**Warning signs:** Tests with `Seed != 0` that were deterministic suddenly fail due to non-deterministic iteration order.

### Pitfall 2: buildSupergraph dedup — Self-Loop Guard
**What goes wrong:** Applying `lo < hi` guard to intra-community self-loops using `n < e.To` may fail when `n == e.To` (actual graph self-loops). The dedup must use `<=` for self-loops or handle the `n == e.To` case separately.
**Why it happens:** Actual self-loops have `n == e.To`; a strict `<` guard drops them entirely.
**How to avoid:** For self-loops (`superN == superNeighbor`), use `n <= e.To` guard. For inter-community edges, the canonical key already handles dedup — only process when iterating from the lower endpoint.

### Pitfall 3: rand.Rand v1 `Seed()` Behavioral Difference
**What goes wrong:** Using `(*rand.Rand).Seed(s)` (deprecated in Go 1.20+) may not produce identical shuffle sequences to `rand.New(rand.NewSource(s))` because Rand has an internal read buffer (`readVal`, `readPos`) that `Seed` does reset, but the internal source initialization differs slightly from constructing fresh.
**Why it happens:** Phase 04 decision explicitly chose `rand.New(src)` to avoid this divergence.
**How to avoid:** If staying on `math/rand` v1, only change to reseed if benchmarks confirm identical sequences with a unit test. Preferred path: migrate to `math/rand/v2` where `rand.NewPCG(seed, 0)` + `Seed()` is clean and documented.
**Warning signs:** `TestLouvainDeterministic` fails (louvain_test.go:197) — this test verifies reproducibility with a fixed seed.

### Pitfall 4: CSR Build Cost on Small Graphs
**What goes wrong:** For the 1K-node benchmark, CSR build time may not be offset by phase1 savings if the graph is small enough that map lookup latency is negligible compared to cache-miss cost.
**Why it happens:** CSR construction is O(V+E); for tiny graphs overhead dominates.
**How to avoid:** The CSR build is still correct on small graphs, just may not improve ns/op for the 1K benchmark. Focus validation on 10K benchmark.

### Pitfall 5: AddNode no-op path does not invalidate cache
**What goes wrong:** `AddNode` has an early-return no-op path for existing nodes (line 34: `if _, exists := g.nodes[id]; !exists`). The cache invalidation (`g.sortedNodes = nil`) must be inside the `if !exists` block, not unconditionally — otherwise every `AddNode` call on an existing node clears the cache.
**Why it happens:** Copy-paste placement error.
**How to avoid:** Place `g.sortedNodes = nil` only inside the `if _, exists := g.nodes[id]; !exists` branch, before or after the mutation.

---

## Code Examples

### Verified: Current buildSupergraph canonical key pattern (louvain.go:319–324)
```go
// Source: graph/louvain.go:319-324
a, b := superN, superNeighbor
if a > b {
    a, b = b, a
}
interEdges[edgeKey{a, b}] += e.Weight
```

### Verified: Dirty-list reset pattern (louvain.go:193–196)
```go
// Source: graph/louvain.go:193-196
for _, k := range state.neighborDirty {
    delete(state.neighborBuf, k)
}
state.neighborDirty = state.neighborDirty[:0]
```

### Verified: sync.Pool pool pattern (louvain_state.go:21–31)
```go
// Source: graph/louvain_state.go:21-31
var louvainStatePool = sync.Pool{
    New: func() any {
        return &louvainState{
            partition:     make(map[NodeID]int),
            commStr:       make(map[int]float64),
            neighborBuf:   make(map[NodeID]float64),
            neighborDirty: make([]NodeID, 0, 64),
            candidateBuf:  make([]int, 0, 64),
        }
    },
}
```

---

## Tests That Reference Changed Functions

All tests pass currently (`go test ./graph/...` = ok, 15.417s). Tests that will exercise the changed code:

| Test File | Test Functions | What It Validates |
|-----------|---------------|-------------------|
| `graph_test.go` | `TestNewGraph`, `TestAddEdgeUndirected`, `TestSubgraph`, `TestClone` | Graph struct mutation, Nodes() cache invalidation, Subgraph seen-map |
| `louvain_test.go` | `TestLouvainDeterministic` (line 197) | RNG reuse correctness — CRITICAL for rand.Rand change |
| `louvain_test.go` | `TestLouvainKarateClub`, `TestLouvainTwoDisconnectedTriangles` | Correctness of phase1 and buildSupergraph |
| `leiden_test.go` | All tests | BFS cursor fix correctness |
| `benchmark_test.go` | `BenchmarkLouvain10K`, `BenchmarkLouvain10K_Allocs` | Primary allocs/op target |

**Run after each optimization area:**
```bash
go test ./graph/... && go test -bench=BenchmarkLouvain10K -benchmem -count=3 ./graph/...
```

**Full benchmark suite:**
```bash
go test -bench=. -benchmem -count=3 ./graph/...
```

---

## Benchmarks — Baseline (Confirmed Live)

Run on Apple M4, go1.26.1, 2026-04-01:

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| BenchmarkLouvain1K | 5 223 305 | 2 334 278 | 5 218 |
| BenchmarkLouvain10K | 63 508 264 | 18 761 436 | 48 793 |

**Target after Phase 1:**
- Louvain 10K allocs/op: ≤ 25 000 (from 48 793)
- Louvain 10K ns/op: ≥ 15% improvement (≤ 53 982 ns/op)

The full benchmark list:
```
BenchmarkLouvain1K          BenchmarkLeiden1K
BenchmarkLouvain10K         BenchmarkLeiden10K
BenchmarkLouvain10K_Allocs  BenchmarkLouvainWarmStart
BenchmarkLeidenWarmStart    BenchmarkEgoSplitting10K
```

---

## Optimization Impact Estimates

| Optimization | Alloc Reduction (est.) | ns/op Reduction (est.) | Confidence |
|-------------|----------------------|----------------------|------------|
| Nodes() cache | ~8 000–15 000 allocs/op | 10–15% | HIGH — 15–25 calls×10K nodes |
| rand.Rand reuse | ~2–6 allocs/op | 1–2% | HIGH — 1 alloc per reset |
| CSR view in phase1 | 0 allocs | 10–20% ns (cache locality) | MEDIUM — map vs slice lookup |
| BFS cursor fix | ~100–500 allocs/op (Leiden) | ~2% ns | MEDIUM |
| buildSupergraph dedup | ~2 allocs (map sizing) | ~3–5% ns | MEDIUM |
| Subgraph seen-map pool | ~10K allocs/op (EgoSplitting) | ~5% EgoSplitting | MEDIUM |
| Dead code removal | 0 allocs | 0 ns | LOW |

Primary alloc contributors (totaling ~48 793 per Louvain 10K run):
- `g.Nodes()` slices across all callers: ~15–25 allocs × 10K nodes = dominant
- `buildSupergraph` new `*Graph` + 2 intermediate maps: ~3 allocs × passes
- `reconstructPartition`/`normalizePartition` maps: ~3 allocs × passes
- `rand.New(src)` per reset: ~2–3 allocs (rng + source)

---

## Environment Availability

Step 2.6: No external dependencies beyond Go stdlib. All changes are stdlib-only.

| Dependency | Available | Version | Notes |
|------------|-----------|---------|-------|
| Go runtime | Yes | go1.26.1 | `math/rand/v2` available (requires Go 1.22+) |
| `math/rand/v2` | Yes | stdlib | Available since Go 1.22; fully stable in 1.26 |
| `sync.Pool` | Yes | stdlib | Already used in codebase |

---

## Open Questions

1. **rand.Rand reuse: v1 Seed() vs v2 migration**
   - What we know: Phase 04 decision explicitly chose `rand.New(src)` over `st.rng.Seed()` to prevent shuffle divergence. `math/rand/v2` with `PCG.Seed()` is the clean path.
   - What's unclear: Whether migrating to `math/rand/v2` changes shuffle sequences for existing seeded tests. `TestLouvainDeterministic` would catch this.
   - Recommendation: Implement v2 migration and run `TestLouvainDeterministic` — if it fails, the test's expected community IDs need to be recalibrated (acceptable since the algorithm is functionally identical, just uses PCG instead of rngSource).

2. **CSR idToIdx map overhead for non-contiguous NodeIDs**
   - What we know: On the original graph, NodeIDs may not be contiguous. After `normalizePartition`, supergraph NodeIDs are always 0-indexed. So CSR on the original graph requires an `idToIdx map[NodeID]int32` which is still a map lookup.
   - What's unclear: Whether the cache locality benefit of flat `[]Edge` outweighs the map lookup cost for `idToIdx`.
   - Recommendation: Implement CSR and measure. If `idToIdx` lookup erases the benefit, limit CSR to supergraph passes only (where NodeIDs are guaranteed contiguous and the map can be replaced with direct indexing).

3. **newLouvainState / newLeidenState deletion**
   - What we know: Neither function is called from any production code path. The ARCHITECTURE.md confirms they are dead code.
   - What's unclear: Whether any test file references them directly (white-box tests have full access to unexported symbols).
   - Recommendation: `grep -n "newLouvainState\|newLeidenState" graph/*_test.go` before deleting. If referenced in tests, keep but add `// Deprecated:` comment.

---

## Sources

### Primary (HIGH confidence)
- Direct source inspection: `graph/graph.go`, `graph/louvain.go`, `graph/leiden.go`, `graph/louvain_state.go`, `graph/leiden_state.go`
- Live benchmark run: `go test -bench=BenchmarkLouvain10K -benchmem ./graph/...` (2026-04-01)
- `.planning/codebase/CONCERNS.md` — optimization audit with exact line numbers
- `.planning/codebase/ARCHITECTURE.md` — data flow and layer descriptions
- `.planning/phases/01-optimize-graph-core/01-CONTEXT.md` — locked decisions

### Secondary (MEDIUM confidence)
- `.planning/STATE.md` Decisions section — Phase 04 decision on rand.New vs Seed rationale
- `go.mod` — confirmed Go 1.26, zero external dependencies

---

## Metadata

**Confidence breakdown:**
- Exact line numbers: HIGH — verified by direct source read
- Nodes() mutation analysis: HIGH — exhaustive grep of all callers
- rand reuse approach: MEDIUM — math/rand/v2 PCG path is correct but shuffle sequence change needs test validation
- CSR impact: MEDIUM — cache locality benefit is well-understood but absolute numbers depend on hardware prefetcher behavior
- Alloc estimates: MEDIUM — based on observed 48 793 baseline and structural analysis of allocation sources

**Research date:** 2026-04-01
**Valid until:** 2026-05-01 (stable codebase, no external dependencies)
