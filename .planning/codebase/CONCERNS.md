# Codebase Concerns — Optimization Analysis

**Analysis Date:** 2026-04-01
**Focus:** Performance bottlenecks, memory allocations, algorithmic complexity, and technical debt impacting throughput.

---

## Top Optimization Points (Ranked by Impact)

| Rank | Area | Estimated Impact | Files |
|------|------|-----------------|-------|
| 1 | Map-based adjacency vs. CSR (slice-of-slices) | High | `graph/graph.go` |
| 2 | `g.Nodes()` allocates fresh `[]NodeID` on every call | High | `graph/graph.go` |
| 3 | `refinePartition` BFS queue uses head-slice dequeue (memory waste) | Medium | `graph/leiden.go` |
| 4 | `buildSupergraph` dual intermediate maps + divide-by-2 correction | Medium | `graph/louvain.go` |
| 5 | `Subgraph` allocates `seen` map per call — called 10K× in EgoSplitting | Medium | `graph/graph.go` |
| 6 | `OmegaIndex` O(n²) all-pairs loop | Medium | `graph/omega.go` |
| 7 | `rand.Rand` re-allocated on every state reset | Low-Medium | `graph/louvain_state.go`, `graph/leiden_state.go` |
| 8 | Warm-start state reset does O(n log n) compaction unconditionally | Low-Medium | `graph/louvain_state.go`, `graph/leiden_state.go` |
| 9 | `normalizePartition` allocates three maps per `Detect` call | Low | `graph/louvain.go` |
| 10 | `computeAffected` iterates neighbor lists redundantly per edge | Low | `graph/ego_splitting.go` |

---

## Performance Bottlenecks

### 1. Map-Based Adjacency — Core Graph Structure

**Problem:** `Graph.adjacency` is `map[NodeID][]Edge`. Every neighbor-list access requires a map lookup (hash + comparison). `g.Neighbors(n)` is called in the innermost loop of `phase1` — once per node per pass — and inside `buildSupergraph`, `refinePartition`, `Strength`, `WeightToComm`, and `buildEgoNet`. Map lookup has worse constants than indexed slice access even at O(1) amortized.

**Files:** `graph/graph.go` (`adjacency map[NodeID][]Edge`, `Neighbors` line 74), called from `graph/louvain.go:phase1`, `graph/leiden.go:refinePartition`, `graph/louvain.go:buildSupergraph`, `graph/ego_splitting.go:buildEgoNet`

**Measured baseline:** Louvain 10K ≈ 62ms/op, 48 773 allocs/op (bench-baseline.txt).

**Cause:** NodeIDs are typed `int` but not guaranteed contiguous at construction time. After `normalizePartition`, supergraph NodeIDs are always 0-indexed contiguous — a CSR representation is directly applicable there.

**Improvement path:**
- For the `phase1` hot path, build a CSR (Compressed Sparse Row) representation once per `Detect` call: `offsets []int32` + `edges []Edge`. Lookup becomes `edges[offsets[n]:offsets[n+1]]` — no hashing, better cache locality.
- The public `Graph` API can remain map-based for construction flexibility; CSR is an internal view built at the start of `Detect`.
- Estimated impact: 20–40% reduction in `phase1` time based on typical map-vs-slice overhead in Go.

---

### 2. `g.Nodes()` — Allocation on Every Call

**Problem:** `graph.go:Nodes()` (line 79) allocates and returns a new `[]NodeID` slice every invocation by iterating `g.nodes` map. It is called in:
- `louvain.go:Detect` — `origNodes := g.Nodes()` plus once per outer pass for `reconstructPartition`
- `louvain_state.go:reset` and `leiden_state.go:reset` — every `phase1` pass reset
- `leiden.go:runOnce` — same
- `modularity.go:ComputeModularityWeighted` — called after every pass for best-Q tracking
- `ego_splitting.go:buildPersonaGraph` — once at startup

For a 10K-node graph with 3–5 Louvain passes, this is 15–25 × 10K-element heap allocations per `Detect` call.

**Files:** `graph/graph.go` (lines 79–85), `graph/louvain.go`, `graph/leiden.go`, `graph/louvain_state.go`, `graph/leiden_state.go`, `graph/modularity.go`

**Improvement path:**
- Cache a sorted `[]NodeID` on the `Graph` struct, invalidated (set to nil) on `AddNode`/`AddEdge`. First call populates the cache; subsequent calls return the cached slice (zero alloc, read-only).
- Add a contract: callers must not modify the returned slice. Document this in the `Nodes()` godoc.
- Alternatively, add `UnsortedNodes(buf []NodeID) []NodeID` that fills a caller-provided buffer, enabling `louvainState.reset` to reuse a pooled slice.

---

### 3. BFS Queue in `refinePartition` — Head-Slice Dequeue

**Problem:** `leiden.go:refinePartition` (line 258) uses `queue = queue[1:]` to dequeue from the front of a slice. This does not shrink the backing array — the head capacity is permanently lost. For a community of k nodes the queue backing array holds k elements; after processing, the array is retained until `refinePartition` returns (escapes to heap). For many small communities, each community gets its own queue allocation that is immediately abandoned.

**Files:** `graph/leiden.go` (lines 255–276)

**Improvement path:**
- Replace head-slice dequeue with a cursor-based approach:
  ```go
  head := 0
  for head < len(queue) {
      cur := queue[head]
      head++
      // process cur...
  }
  ```
  Same O(k) memory, zero additional allocations, no abandoned capacity.
- Pool the queue buffer across communities via `sync.Pool` or pass a reusable slice into `refinePartition`, resetting `head=0` and `queue=queue[:0]` per community.

---

### 4. `buildSupergraph` — Dual Intermediate Maps and Division Corrections

**Problem:** `louvain.go:buildSupergraph` (lines 304–339) accumulates all intra-community edges into `selfLoops map[NodeID]float64` and all inter-community edges into `interEdges map[edgeKey]float64`. Because undirected edges appear in adjacency from both endpoints, both maps accumulate double-counted weights that are then halved at write time (`w/2.0`). This requires:
1. Two full O(E) passes over all edges
2. Two intermediate map allocations
3. A division per map entry at write time

**Files:** `graph/louvain.go` (lines 272–348)

**Improvement path:**
- Canonicalize during accumulation: use a `lo < hi` guard when iterating neighbors to process each undirected edge only once. This eliminates the divide-by-2 correction and halves map entries.
- Pre-size maps: `make(map[edgeKey]float64, g.EdgeCount())` to avoid repeated rehashing as the map grows.
- Combine `selfLoops` and `interEdges` into a single map keyed by `[2]NodeID` (self-loops use `{super, super}`) to reduce the number of map operations.

---

### 5. `Subgraph` — Per-Call `seen` Map Allocation

**Problem:** `graph.go:Subgraph` (line 204) allocates `seen := make(map[[2]NodeID]struct{})` on every call to deduplicate undirected edges. `buildEgoNet` in `ego_splitting.go` (line 432) calls `g.Subgraph(neighborIDs)` once per node during `buildPersonaGraph`. For a 10K-node graph, this is 10K separate map allocations, one per ego-net build.

**Files:** `graph/graph.go` (lines 185–230), `graph/ego_splitting.go:buildEgoNet` (line 432)

**Improvement path:**
- For the ego-net use case, neighbor lists are small (avg degree ≈ 5 for BA(m=5) graphs). Use a sorted-slice intersection approach instead of a hash map for edge deduplication when `len(nodeIDs) < threshold`.
- Expose `SubgraphInto(nodeIDs []NodeID, dst *Graph)` that writes into a pre-allocated `Graph`, avoiding a `NewGraph` allocation per call.
- Pool the `seen` map via `sync.Pool` and clear it before/after use.

---

### 6. `OmegaIndex` — O(n²) All-Pairs Loop

**Problem:** `omega.go:OmegaIndex` (line 70) iterates all `n*(n-1)/2` node pairs. For n=10K, this is ~50M comparisons. Each pair calls `countSharedMemberships` which iterates the smaller membership set. Currently only used in accuracy tests, not in the `Detect` hot path.

**Files:** `graph/omega.go` (lines 70–87)

**Current complexity:** O(n² × avg_communities_per_node)

**Improvement path:**
- Rewrite using co-occurrence counting: for each community c with members S_c, for each pair (u,v) in S_c×S_c, increment `tResult[{u,v}]++`. Complexity becomes O(Σ |S_c|²) — for sparse community structures (small communities), this is far less than O(n²).
- For production use on large graphs, add a Monte Carlo sampling mode: sample k random pairs and estimate Omega from the sample.

---

### 7. `rand.Rand` Re-Allocated on Every State Reset

**Problem:** `louvain_state.go:reset` (line 65) and `leiden_state.go:reset` (line 70) both call `rand.New(src)` on every invocation, allocating a new `*rand.Rand`. The state structs are pooled via `sync.Pool` to reduce GC pressure, but the `rng` field is replaced on every reset — negating the pool benefit for this field. For Leiden with default 3 runs, this is 3 × `rand.New` allocations per `Detect` call.

**Files:** `graph/louvain_state.go` (line 65), `graph/leiden_state.go` (line 70)

**Improvement path:**
- Store a `rand.Source` in the state struct and re-seed it via `src.(*rand.rngSource).Seed(newSeed)` rather than constructing a new `rand.Rand`. The `rand.Rand` wrapper can be allocated once during pool construction and reused.
- Or migrate to `math/rand/v2` (available since Go 1.22) which supports `rand.New(rand.NewPCG(seed, 0))` with a cheaper seed reset path.

---

## Technical Debt

### Dead Code: `deltaQ` Standalone Function

**Issue:** `louvain.go:deltaQ` (lines 261–270) is a standalone exported-style helper that computes ΔQ by calling `g.WeightToComm` and `g.Strength`. The `phase1` hot path inlines this computation directly using precomputed buffers (`state.neighborBuf`, `state.commStr`) — `deltaQ` is not called anywhere in production code. It remains as dead code after the `phase1` optimization.

**Files:** `graph/louvain.go` (lines 261–270)

**Risk:** None — purely dead code. No correctness risk.

**Fix:** Delete `deltaQ`. Its logic is preserved inline in `phase1` (lines 229–248).

---

### `CommStrength` — O(n) Full Partition Scan, Unused in Hot Path

**Issue:** `graph.go:CommStrength` (lines 244–252) iterates the entire `partition` map to sum strengths for one community. This is O(n) per call. The hot path in `phase1` uses the precomputed `state.commStr` cache instead. `CommStrength` exists as a public method but is only exercised in `graph_test.go` (line 179).

**Files:** `graph/graph.go` (lines 244–252)

**Risk:** Low for current code. Risk increases if future callers use `CommStrength` inside inner loops.

**Fix:** Add a `// O(n): do not call inside inner loops — use a precomputed commStr cache instead` comment. Consider deprecating or removing the method if it has no planned external use.

---

### Warm-Start Compaction — O(n log n) Run on Every Reset

**Issue:** `louvain_state.go:reset` (lines 99–115) and `leiden_state.go:reset` (same pattern, lines 99–115) perform a full community-ID compaction to 0-indexed contiguous integers on every warm-start invocation. This involves a full pass over all nodes (already sorted) plus a remap pass — O(n) work with non-trivial constant. For Leiden's default 3-run loop, this runs 3 times at the start of each `runOnce` call.

**Files:** `graph/louvain_state.go` (lines 99–115), `graph/leiden_state.go` (lines 99–115)

**Impact:** Warm-start speedup is currently 1.2×–1.4× over cold-start (bench-baseline.txt). Eliminating the compaction could recover 5–10% of the warm-start call overhead.

**Fix approach:** Skip compaction when `InitialPartition` is known to already be 0-indexed contiguous (e.g., produced by a prior `normalizePartition` call). Document a `contiguous bool` contract or add a `normalizeBeforeWarm bool` option.

---

### `louvainState` Aliasing Inside `leiden.go`

**Issue:** `leiden.go:runOnce` (lines 138–145) constructs a temporary `*louvainState` that aliases fields from `leidenState` in order to pass it to `phase1(g, ls, ...)`. After `phase1` returns, four fields are manually synced back (lines 147–150). This is a structural workaround because `phase1` accepts `*louvainState` rather than an interface.

**Files:** `graph/leiden.go` (lines 138–150)

**Risk:** Low for correctness today. Fragile under refactor — any new field added to `louvainState` that `phase1` writes must be manually added to both the alias construction and the sync-back.

**Fix approach:** Define a `phaseState` interface or a shared embedded struct (`basePhaseState`) that both `louvainState` and `leidenState` embed. Pass the interface/embedded struct to `phase1` to eliminate the aliasing pattern.

---

### `Tolerance` Field Defined but Never Read

**Issue:** `LouvainOptions.Tolerance` and `LeidenOptions.Tolerance` are defined in `detector.go` (lines 33, 46) with documented defaults of `1e-7`. Neither `louvain.go` nor `leiden.go` reads `d.opts.Tolerance` anywhere. Termination is solely `moves == 0`. Callers setting `Tolerance` receive no effect — a silent API contract violation.

**Files:** `graph/detector.go` (lines 33, 46), `graph/louvain.go`, `graph/leiden.go`

**Impact:** Beyond the correctness gap, tolerance-based early termination would be a real performance win: for graphs where `moves` oscillates near 0 but never reaches it, adding a ΔQ-gain threshold check would allow early convergence without quality loss.

**Fix approach:** Either implement tolerance-based termination in `phase1` (check cumulative ΔQ gain per pass against `opts.Tolerance * currentQ`), or remove the field from the public API with a deprecation notice.

---

## Memory Usage Patterns

### Allocation Profile (Apple M4, from bench-baseline.txt)

| Benchmark | ns/op | B/op | allocs/op |
|-----------|-------|------|-----------|
| Louvain 1K | ~5.4ms | 2.33 MB | 5 217 |
| Leiden 1K | ~5.5ms | 2.58 MB | 7 248 |
| Louvain 10K | ~62ms | 18.6 MB | 48 773 |
| Leiden 10K | ~67ms | 21.0 MB | 66 524 |
| Louvain WarmStart 10K | ~46ms | 10.3 MB | 25 754 |
| Leiden WarmStart 10K | ~51ms | 13.2 MB | 37 133 |

**Key observations:**
- Louvain 10K produces ~48K allocations. Scale factor 1K→10K is ~9.4× (slightly super-linear), suggesting per-pass overhead grows faster than O(n).
- Leiden adds ~18K allocs vs. Louvain — attributable to `refinePartition` per pass: `commNodes` map, `inComm` map, `visited` map, and BFS `queue` slice per community per pass.
- Warm-start reduces allocs by ~47% for Louvain (25 754 vs. 48 773) — consistent with fewer passes needed. The remaining 25K allocs are structural (supergraph builds, `g.Nodes()` slices, partition maps).
- The dominant per-pass allocation sources: `buildSupergraph` (new `*Graph` + 2 intermediate maps), `g.Nodes()` slices, `reconstructPartition` + `normalizePartition` maps.

---

## Concurrency

### Ego-Net Parallelism — Channel Overhead at Scale

**Current:** `buildPersonaGraph` and `buildPersonaGraphIncremental` use a bounded goroutine pool sized to `runtime.GOMAXPROCS(0)` and dispatch jobs one-per-message via a buffered channel (`cap = workerCount*2`). For 10K nodes and 8 workers, the producer goroutine sends 10K channel messages.

**Files:** `graph/ego_splitting.go` (lines 397–421, `runParallelEgoNets`; lines 497–503, dispatch goroutine)

**Estimated channel overhead:** ~40–100 ns/send × 10K sends ≈ 0.4–1.0ms. Small relative to the ~230ms total EgoSplitting time, but measurable.

**Improvement path:** Batch jobs into chunks of `len(nodes)/workerCount` and send one slice per worker instead of one message per node. Eliminates per-node channel overhead while preserving parallelism.

---

### `sync.Pool` Shared Across All Concurrent `Detect` Calls

**Current:** `louvainStatePool` and `leidenStatePool` are package-level singletons. Go's `sync.Pool` is per-P (processor), so contention is only an issue when goroutine count >> GOMAXPROCS.

**Files:** `graph/louvain_state.go` (line 21), `graph/leiden_state.go` (line 23)

**Scaling concern:** In a server environment with high concurrency (many parallel `Detect` calls), pool thrashing could occur when P-local free lists are exhausted and cross-P steals happen. At current scale (unit test / benchmark), this is not a concern.

---

## Scaling Limits

### EgoSplitting: Persona Graph Size Grows Super-Linearly with Avg Degree

**Current:** For a BA(m=5) graph (avg degree 10), each node produces ~1–2 personas, yielding a persona graph ~9.4× larger than the original (10K nodes → ~94K persona nodes, per benchmark_test.go line 184). GlobalDetector runs Louvain on this 94K-node persona graph with `MaxPasses=1`.

**Scaling concern:** For denser graphs (BA(m=10), avg degree 20), nodes may split into 3–5 personas, growing the persona graph to 30–50K from 10K original nodes — a 3–5× larger GlobalDetector workload. The `MaxPasses=1` optimization mitigates but does not eliminate this.

**Files:** `graph/ego_splitting.go:buildPersonaGraph`, `graph/benchmark_test.go` (line 184)

### Louvain/Leiden: Supergraph Compression Rate

**Current:** If Phase 1 moves are few (near convergence), `buildSupergraph` produces a graph nearly as large as the current graph — low compression, high per-pass overhead. The outer loop in `louvain.go:Detect` continues until `moves == 0` with no compression-ratio guard.

**Improvement path:** Add an early-exit condition: if supergraph node count > `threshold * currentGraph.NodeCount()` (e.g., 0.9), convergence is effectively achieved and further passes add overhead without quality gain. This is related to the unused `Tolerance` field described above.

---

*Concerns audit: 2026-04-01*
