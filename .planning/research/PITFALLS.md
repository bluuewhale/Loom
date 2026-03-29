# Domain Pitfalls: Louvain + Leiden Community Detection in Go

**Domain:** Graph community detection (Louvain + Leiden) for a Go GraphRAG library
**Researched:** 2026-03-29
**Overall confidence:** HIGH (algorithm correctness) / HIGH (Go-specific) / MEDIUM (NMI accuracy)

---

## Part 1 — Algorithm Correctness Pitfalls

These mistakes produce wrong Q values, wrong partitions, or silent infinite loops.

---

### CRIT-01: Self-Loop Double-Counting in Phase 2 Supergraph

**What goes wrong:** When building the supergraph (phase 2 aggregation), intra-community edges are collapsed into a self-loop on the supernode. If you add self-loop weight using the same `AddEdge` path as regular edges, subsequent phase-1 passes will count that self-loop's weight as contributing to `k_i_in` when computing ΔQ, inflating the gain for nodes merging into that community.

**Root cause:** The standard ΔQ formula sums `k_i_in` — the weight of edges from node `i` to nodes already in the target community. A self-loop on `i` itself has both endpoints in the same node, so it is always in the same community. Including it in `k_i_in` is incorrect — it is already baked into `Σ_in`.

**Specific failure:** k_i_in calculation must exclude self-loop edges. Any loop over `g.Neighbors(u)` used to accumulate neighbor-community weights must skip edges where `neighbor == u`.

**Consequences:** Modularity Q appears inflated. Nodes will merge too aggressively, producing over-merged communities with falsely high Q values. Karate Club test may still pass if the Q threshold is loose.

**Prevention:**
```go
// In neighbor accumulation loop — REQUIRED guard:
for _, e := range g.Neighbors(u) {
    if e.To == u { // skip self-loops
        continue
    }
    // accumulate neighborWeightBuf[assignment[e.To]] += e.Weight
}
```

**Detection:** On a graph where phase 2 has been run at least once (any multi-pass run), compute Q with `ComputeModularity` and compare to the value tracked internally. Divergence > 1e-9 indicates double-counting.

---

### CRIT-02: Incorrect ΔQ Formula — Missing Factor of 2 for Undirected Graphs

**What goes wrong:** The standard Louvain ΔQ for moving node `i` into community `C` is:

```
ΔQ = [k_i,in / m] - [Σ_tot * k_i / (2m²)]
```

where `m` is the **sum of all edge weights** (each undirected edge counted once). A frequent mistake is computing `2m` by iterating edges and counting each undirected edge twice (once per endpoint), producing `m = total_edge_weight_sum * 2`. This makes the first term twice as large as it should be, causing nodes to merge too eagerly on the first pass.

**Root cause:** Confusion between the adjacency matrix convention (`A_ij + A_ji` for undirected, so `Σ A_ij = 2m`) and the actual total edge weight (`m`). The `totalWeight` field in `louvainState` must be `Σ edge.Weight` over unique edges, not over all directed half-edges.

**Specific failure:** If `Graph.directed == false` and `AddEdge(u, v, w)` stores the edge in both `adj[u]` and `adj[v]`, then iterating `adj` to sum weights produces `2m`. You must divide by 2, or iterate only once (one direction).

**Prevention:**
- Compute `totalWeight` by iterating `g.Edges()` (unique edges), not `g.Neighbors` (which iterates half-edges for undirected graphs).
- Assert: `ComputeModularity(g, singletonPartition) ≈ 0.0` — if the formula returns non-zero for a partition where every node is in its own community, the normalisation is wrong.

**Detection:** Unit test: single undirected edge (u—v, weight=1.0) with two-community partition `{u:0, v:1}`. Q must be exactly `-0.5`. Wrong factor of 2 produces `-0.25` or `-1.0`.

---

### CRIT-03: Resolution Parameter Applied to Wrong Term

**What goes wrong:** The resolution-parameterised modularity (Reichardt-Bornholdt) is:

```
Q_γ = (1/2m) Σ_{ij} [A_ij - γ * k_i*k_j / (2m)] δ(c_i, c_j)
```

A common mistake is applying γ to the entire expression instead of only to the null-model term, or computing `k_i * k_j * γ / (2m)` but using the raw degree `k_i` instead of the weighted degree (strength) when the graph is weighted.

**Consequences:** γ > 1 should produce more communities. If γ is applied incorrectly, it may have the opposite effect or no effect. The algorithm will appear to run but produce partitions identical to γ=1.0 regardless of the parameter value.

**Prevention:**
- ΔQ formula: `ΔQ = k_i_in/m - γ * (Σ_tot * k_i) / (2m²)`. The γ multiplies only the second term.
- Use `g.Strength(u)` (sum of edge weights) for `k_i`, not `g.Degree(u)` (edge count), for weighted graphs.
- Test: run Karate Club with γ=2.0. Community count should be strictly greater than with γ=1.0.

---

### CRIT-04: Phase-1 Restart Logic — Restarting Too Eagerly vs Not Enough

**What goes wrong:** There are two different "restart" interpretations of Louvain phase 1:

**Variant A (correct — original Blondel 2008):** Iterate over all nodes in a random order. After visiting every node once, if any node moved during this pass, immediately start a new pass (full re-scan with new random order). Stop when a full pass produces zero moves.

**Variant B (common mistake — "single-pass" termination):** Visit all nodes once; stop. This converges in 1 pass but produces much lower modularity because early-moved nodes are never re-evaluated given their new neighbours.

**Variant C (common mistake — "continuous cycling"):** After moving a node, immediately re-visit it again in the same pass rather than continuing to the next node. This can cause oscillation: node `i` moves to community A, then immediately back to B, looping indefinitely if ΔQ(A) ≈ ΔQ(B).

**Correct behaviour:**
1. Shuffle node visit order at the start of each pass.
2. Visit each node exactly once per pass; move it to best community (ΔQ > tolerance) or leave it.
3. After all N nodes visited: if `moves > 0`, increment pass counter and start a new pass.
4. Stop when a complete pass produces `moves == 0` OR `passes >= MaxPasses`.

**Prevention:** Track `moves` counter per pass (not globally). Reset to 0 at the start of each pass. The pass-termination condition is `moves == 0` after a full sweep, not `ΔQ_sum < tolerance` (those are different convergence criteria — per-pass move count is the correct one for preventing Variant B/C).

---

### CRIT-05: Tiny ΔQ Values Causing Infinite Loop (Confirmed Go Bug — gonum/gonum #1488)

**What goes wrong:** If `ΔQ > 0` but `ΔQ ≈ 2.7e-21` (floating-point noise), the node moves on every pass. But the move changes nothing observable about the community structure (single float64 accumulated in Σ_tot), so the next pass also finds the same positive ΔQ, creating an infinite loop.

**Root cause:** `ΔQ > 0.0` strict comparison accepts floating-point noise as a valid improvement. This was observed in the gonum Louvain implementation and caused confirmed infinite loops.

**Prevention:**
```go
const defaultTolerance = 1e-7

// Use tolerance-guarded comparison everywhere:
if deltaQ > opts.Tolerance {
    // move node
}
```

The tolerance must be the same value used in the convergence check. Using 0.0 as the move threshold while using 1e-7 as the convergence threshold creates a gap where single-node micro-moves never trigger convergence.

**Detection:** Run Louvain with `MaxPasses: 100` on a complete graph K_8. Should converge in ≤ 3 passes. More than 10 passes indicates oscillation.

---

### CRIT-06: Supergraph Phase 2 — Not Preserving Internal Edge Weights as Self-Loops

**What goes wrong:** When collapsing community C into a supernode, the sum of all edges **within** C must be stored as a self-loop on the supernode. If you simply skip intra-community edges during supergraph construction (treating them as "already merged"), the Σ_in values in the next phase's `louvainState` will be initialised to 0, as if each supernode started with no internal edges.

**Consequences:** Phase 1 of the next level will compute ΔQ as if supernodes have zero internal weight. Nodes will merge too eagerly, possibly collapsing the entire graph into one community after the second pass.

**Correct behaviour:**
```go
// In buildSupergraph:
for _, e := range g.Neighbors(u) {
    cu := state.assignment[u]
    cv := state.assignment[e.To]
    if cu == cv {
        // Intra-community: accumulate into self-loop weight for supernode
        internalWeightMap[cu] += e.Weight // will become self-loop
    } else {
        // Cross-community: add as regular edge in supergraph
        supergraph.AddEdge(communityMap[cu], communityMap[cv], e.Weight)
    }
}
// After edge loop: add self-loops
for cid, w := range internalWeightMap {
    supergraph.AddEdge(communityMap[cid], communityMap[cid], w)
}
```

**Detection:** After phase 2, `ComputeModularity(supergraph, identityPartition)` should equal `ComputeModularity(original, phase1Partition)`. Any difference > 1e-9 means internal weight was lost.

---

### CRIT-07: Directed Graph — Using Undirected ΔQ Formula

**What goes wrong:** For directed graphs, modularity is defined as:

```
Q = (1/m) Σ_{ij} [A_ij - k_i^out * k_j^in / m] δ(c_i, c_j)
```

where `k_i^out` and `k_j^in` are separate out-degree and in-degree sums. Applying the undirected formula (which uses `k_i * k_j / (2m)`) to a directed graph systematically underestimates the null model for nodes with asymmetric degree, producing wrong communities.

**Prevention:** The `louvainState` must track separate `outDegree` and `inDegree` slices for directed graphs. The ΔQ computation is a different code path. Add a branch at the top of phase 1: `if g.directed { directedDeltaQ(...) } else { undirectedDeltaQ(...) }`.

**Detection:** On a directed ring graph (each node points to the next), standard modularity should detect the ring structure. Applying undirected formula to a directed ring will find fewer communities than expected.

---

### CRIT-08: Leiden Refinement — Selective Refinement Causes Disconnected Communities

**What goes wrong:** A known flaw (documented in arxiv 2402.11454, 2024): when refinement is applied only to a subset of communities (e.g., communities that changed in the local-moving phase), a vertex migration can disconnect the original community. The destination community gains a new node; the source community may have its connectivity broken.

**Root cause:** Leiden's connectivity guarantee holds only when all communities are refined after each local-moving phase, not just the changed ones.

**Prevention:**
- After local-moving, always refine ALL communities, not just those with `dirty` nodes.
- After any vertex moves during refinement, queue the source community for a connectivity check.
- Assert at the end of each Leiden iteration: for every community, all member nodes are reachable within the subgraph induced by that community.

**Detection:**
```go
// After each Leiden iteration — test helper:
func assertConnectedCommunities(g *Graph, partition Partition) error {
    // group nodes by community, BFS/DFS within each group,
    // return error if any group has unreachable nodes
}
```

---

### CRIT-09: NMI Accuracy Failure Modes on Specific Graph Types

| Graph Type | Failure Mode | Root Cause | Mitigation |
|------------|-------------|------------|-----------|
| Bipartite graphs | Standard Louvain finds wrong communities | Modularity null-model assumes unipartite degree distribution | Use bipartite-specific modularity or project to unipartite first |
| Regular graphs (all equal degree) | Any partition has Q ≈ 0; algorithm produces random communities | All ΔQ values near 0; float noise drives decisions | Not a bug — document limitation; use Leiden with γ > 1.0 |
| Very small graphs (< 10 nodes) | NMI ≥ 0.90 threshold is too strict; community structure is ambiguous | Resolution limit: Q cannot distinguish small sub-communities | Use exact test (expected community count) rather than NMI threshold |
| High-mixing LFR (µ ≥ 0.4) | NMI drops sharply below 0.7 | Community boundaries are inherently blurry | Expected behaviour — document; do not assert NMI ≥ 0.9 for µ > 0.3 |
| Disconnected graphs (no edges) | Q = 0 / NaN | 0 total edge weight makes `1/2m` undefined | Guard: `if m == 0 { return singletonPartition, nil }` |

**Resolution limit (Fortunato & Barthélemy, PNAS 2007):** Communities smaller than `√(2m)` nodes in size cannot be resolved by modularity maximisation, regardless of algorithm. This is a theoretical bound, not an implementation bug. Do not write tests asserting fine-grained community detection on small sub-structures within large dense graphs.

---

## Part 2 — Performance and Benchmarking Pitfalls

---

### PERF-01: Benchmarking Setup Cost Inside the Timing Loop

**What goes wrong:** Including graph construction (`NewGraph`, `AddEdge` calls) inside the `b.N` loop measures I/O and memory allocation, not the algorithm.

**Prevention:**
```go
func BenchmarkLouvain10K(b *testing.B) {
    g := buildSyntheticGraph(10_000, 50_000) // outside loop
    det := NewLouvain(LouvainOptions{Seed: 42})
    b.ResetTimer()
    for b.Loop() { // Go 1.24+ b.Loop() — handles iteration count automatically
        _, _ = det.Detect(g)
    }
}
```

**Go 1.24 note:** Use `b.Loop()` instead of `for i := 0; i < b.N; i++`. It avoids the timer-reset pitfalls of `b.StopTimer`/`b.StartTimer` and correctly handles first-iteration warmup.

---

### PERF-02: Compiler Dead-Code Elimination Removing the Measured Work

**What goes wrong:** If the return value of `Detect` is discarded without use, the Go compiler (at high optimisation levels) may elide the call entirely, producing 0ns/op benchmarks.

**Prevention:**
```go
var sinkPartition Partition
var sinkErr error

for b.Loop() {
    sinkPartition, sinkErr = det.Detect(g)
}
// Prevent elimination:
_ = sinkPartition
_ = sinkErr
```

Alternatively use `testing.AllocsPerRun` to verify the result is materialised.

---

### PERF-03: sync.Pool Contamination Between Benchmark Runs

**What goes wrong:** `sync.Pool` reuses `louvainState` across `Detect` calls. If `st.reset()` does not fully zero the slice contents (e.g., only resets `len` not values), a subsequent call on a different graph will start with stale community assignments. This produces correct results if graphs are the same size but wrong results for mixed-size benchmarks.

**Prevention:**
```go
func (st *louvainState) reset() {
    // Zero all slices — do not just reslice to length 0
    for i := range st.assignment { st.assignment[i] = 0 }
    for i := range st.totalDegree { st.totalDegree[i] = 0 }
    for i := range st.internalWeight { st.internalWeight[i] = 0 }
    // neighborWeightBuf is zeroed per-node in the inner loop via dirty list —
    // but must be fully zeroed here because dirty list from previous call may be shorter
    for i := range st.neighborWeightBuf { st.neighborWeightBuf[i] = 0 }
}
```

**Detection:** Run `BenchmarkLouvain_MixedSizes` that alternates 100-node and 10K-node graphs. Assert modularity Q of returned partition matches `ComputeModularity` on every iteration.

---

### PERF-04: Measuring GC Pressure Instead of Algorithm Latency

**What goes wrong:** If benchmarks are run without `-benchmem` and without checking allocs, the 100ms target may be met only because GC pauses happen to fall outside the measured window. Subsequent production use (with many concurrent goroutines) degrades due to GC contention.

**Prevention:**
- Always run benchmarks with `-benchmem` and assert `0 allocs/op` after Phase 1 state is pooled.
- Use `runtime.ReadMemStats` to verify zero heap growth per iteration in steady state.
- The `sync.Pool` pattern (ARCHITECTURE.md) should bring allocations to 0 after first call; verify this with `-benchmem`.

---

### PERF-05: RunParallel Benchmark Using Global Timer Controls

**What goes wrong:** Using `b.StartTimer`/`b.StopTimer` inside `b.RunParallel` has global effect — it affects all parallel goroutines' timing, producing non-monotonic or zero elapsed times.

**Prevention:** Do not use `b.StartTimer`/`b.StopTimer` inside `RunParallel`. Use `b.Loop()` which handles timing correctly. For concurrent safety benchmarks:

```go
func BenchmarkLouvainConcurrent(b *testing.B) {
    g := buildSyntheticGraph(10_000, 50_000)
    det := NewLouvain(LouvainOptions{Seed: 42})
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            _, _ = det.Detect(g) // no timer manipulation here
        }
    })
}
```

---

## Part 3 — Go Concurrency Pitfalls

---

### CONC-01: Storing Phase State in LouvainDetector Fields

**What goes wrong:** If `louvainState` is stored as a field on `LouvainDetector` (e.g., for "convenience" reuse), two concurrent `Detect` calls on the same detector will share and corrupt each other's assignment slices.

**Prevention:** `LouvainDetector` must be immutable after construction. All mutable state (assignment, totalDegree, etc.) must be allocated per `Detect` call or fetched from `sync.Pool`. The detector struct must hold only `LouvainOptions`.

**Detection:** `go test -race` on a test that calls `det.Detect(g)` concurrently from 4 goroutines. Any data race indicates state leakage into the receiver.

---

### CONC-02: Concurrent Reads of *Graph Are Safe Only If Caller Does Not Write

**What goes wrong:** `Detect` reads `*Graph` without locks (correct — reads are concurrent-safe for read-only access). But if a caller modifies the graph (calls `AddEdge`, `RemoveEdge`) while a `Detect` is in progress on the same `*Graph`, the adjacency `map` read in `g.Neighbors` will race with the write.

**Prevention:** Document the contract explicitly in code:
```go
// Detect reads g but never writes to it. Concurrent Detect calls on distinct
// *Graph values are safe. Concurrent Detect + graph mutation on the SAME *Graph
// is NOT safe — the caller must synchronise externally.
func (l *LouvainDetector) Detect(g *Graph, ...) ...
```

Do not add a `sync.RWMutex` inside `*Graph` — it would serialise all concurrent detects sharing the same graph, which is a correctness choice the caller should make. Adding it inside `*Graph` hides the contract and adds overhead for the common case (one graph per goroutine).

---

### CONC-03: sync.Pool Returning Oversized State for Smaller Graphs

**What goes wrong:** `sync.Pool` may return a `louvainState` whose backing arrays were sized for a 10K-node graph when the current call is on a 100-node graph. If `st.init` only reslices (`st.assignment = st.assignment[:numNodes]`) without zeroing, the 100-node call sees zeros in valid positions but the `neighborWeightBuf` (sized at 10K) still has stale values from the last call's dirty-list flush. If the dirty-list tracking fails to cover all touched indices, stale values produce wrong ΔQ.

**Prevention:** In `st.init`, always zero-fill slices up to `numNodes` regardless of previous length:
```go
func (st *louvainState) init(g *Graph) {
    n := g.Len()
    // Grow backing arrays if needed
    if cap(st.assignment) < n { st.assignment = make([]int, n) }
    st.assignment = st.assignment[:n]
    // Zero the active region explicitly
    clear(st.assignment) // Go 1.21+ builtin clear
    clear(st.totalDegree[:n])
    // neighborWeightBuf must be zeroed to len(neighborWeightBuf), not len n,
    // because stale entries outside [0,n) could be accessed if communityIDs
    // are not dense — safer to zero the full buffer
    clear(st.neighborWeightBuf)
}
```

---

### CONC-04: Random Source Sharing Across Goroutines

**What goes wrong:** If a single `*rand.Rand` (or `rand.Shuffle`) is shared across concurrent `Detect` calls (e.g., stored as a package-level var), concurrent goroutines will race on the RNG state.

**Prevention:**
- With `Seed == 0`: use `rand.Shuffle` from the `math/rand/v2` global source (Go 1.22+ global is goroutine-safe by default).
- With `Seed != 0`: create a `*rand.Rand` local to each `Detect` call from the provided seed. Do not store it on the detector.

```go
func (l *LouvainDetector) Detect(g *Graph, opts DetectOptions) (Partition, error) {
    var rng *rand.Rand
    if opts.Seed != 0 {
        rng = rand.New(rand.NewSource(opts.Seed)) // local, not shared
    }
    // use rng.Shuffle or global rand.Shuffle(len, swap) if rng == nil
}
```

**Detection:** `go test -race` with `Seed: 42` and 4 concurrent goroutines on the same detector.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| LouvainDetector — phase 1 inner loop | CRIT-01 (self-loop in k_i_in), CRIT-05 (tiny ΔQ infinite loop) | Guard self-loop skip; use tolerance comparison |
| Phase 2 supergraph construction | CRIT-06 (missing self-loop preservation), CRIT-02 (2m factor) | Always add intra-community weight as self-loop on supernode |
| Weighted directed graph support | CRIT-07 (wrong formula) | Branch on `g.directed` before ΔQ computation |
| Resolution parameter (γ) | CRIT-03 (γ applied to wrong term) | Unit test: γ=2.0 must produce more communities than γ=1.0 |
| Leiden refinement phase | CRIT-08 (selective refinement = disconnected communities) | Refine all communities; assert connectivity after each iteration |
| Benchmark fixtures | PERF-01, PERF-02 | Build graph outside loop; use `b.Loop()`; sink return values |
| sync.Pool state reuse | PERF-03, CONC-03 | Use `clear()` in `st.init` for full zero-fill |
| Concurrent detector use | CONC-01, CONC-04 | State on stack / pool; per-call RNG; document read-only graph contract |

---

## Sources

- [gonum/gonum issue #1488 — infinite loop on tiny deltaQ in Louvain](https://github.com/gonum/gonum/issues/1488) — confirmed real Go Louvain bug
- [Louvain-Performance and Self-Loops (Pomona Complex Networks)](https://research.pomona.edu/complexnetworks/2016/01/03/louvain-performance-and-self-loops-the-problem/) — self-loop accounting in performance metric
- [Sotera spark-louvain issue #1 — k_i_in self-loop bug](https://github.com/Sotera/spark-distributed-louvain-modularity/issues/1) — cross-implementation confirmation
- [From Louvain to Leiden — Traag et al., Scientific Reports 2019](https://www.nature.com/articles/s41598-019-41695-z) — Leiden correctness guarantees, refinement phase
- [Addressing Internally-Disconnected Communities in Leiden (arxiv 2402.11454)](https://arxiv.org/html/2402.11454v2) — selective refinement disconnection bug
- [GVE-Louvain: Fast Louvain (arxiv 2312.04876)](https://arxiv.org/html/2312.04876v4) — tolerance-based convergence, neighbor accumulator
- [Fast Louvain — Delta Modularity formula](https://splines.github.io/fast-louvain/louvain/delta-modularity.html) — correct ΔQ derivation
- [Resolution limit in community detection — Fortunato & Barthélemy, PNAS 2007](https://www.pnas.org/doi/10.1073/pnas.0605965104) — theoretical NMI accuracy limit
- [Common pitfalls in Go benchmarking — Eli Bendersky](https://eli.thegreenplace.net/2023/common-pitfalls-in-go-benchmarking/) — compiler elimination, setup cost
- [More predictable benchmarking with testing.B.Loop — go.dev blog](https://go.dev/blog/testing-b-loop) — Go 1.24 b.Loop() guidance
- [Maximizing modularity in directed networks (HAL)](https://hal.science/hal-01784/document) — directed modularity formula
- [Louvain method — Wikipedia](https://en.wikipedia.org/wiki/Louvain_method) — phase definitions, node visit order

---

*Pitfalls research: 2026-03-29*
