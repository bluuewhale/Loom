# Phase 05: Warm Start — Research

**Researched:** 2026-03-30
**Domain:** Incremental / online community detection — seeding Louvain and Leiden from a prior partition
**Confidence:** HIGH (all findings grounded in direct codebase analysis; algorithmic claims from established literature)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01:** Extend `LouvainOptions` and `LeidenOptions` with `InitialPartition map[NodeID]int`. Nil = cold start. No breaking change.
- **D-02:** Warm start is applied inside `reset()` in `louvain_state.go` and `leiden_state.go`. When `opts.InitialPartition != nil`, populate `state.partition` from it instead of trivial `i → i`. `commStr` is recomputed from the seeded partition.
- **D-03:** New nodes (in graph but not in prior partition) → assign singleton community starting after `max(prior community IDs) + 1`.
- **D-04:** Removed nodes (in prior partition but not in graph) → silently ignored; reset loop only iterates `g.Nodes()`.
- **D-05:** After seeding, re-compact community IDs using `normalizePartition` (or inline equivalent). Keeps 0-indexed contiguous invariant.
- **D-06:** Convergence criteria unchanged — same `Tolerance`, `MaxPasses`/`MaxIterations`.
- **D-07:** `leidenState.refinedPartition` is left nil on warm start. Populated during first BFS refinement pass as usual.
- **D-08:** Add `BenchmarkLouvainWarmStart` and `BenchmarkLeidenWarmStart` in `benchmark_test.go`. Scenario: cold detect on 10K-node graph, then ±1% edge perturbation, compare warm vs cold ns/op. Target: warm ≤ 50% of cold ns/op.
- **D-09:** Accuracy tests: Q(warm) ≥ Q(cold) on three existing fixtures after small perturbation. Also: warm start on unperturbed graph converges in fewer passes than cold.

### Claude's Discretion

- How to pass `InitialPartition` down from the detector to `reset()`: cleanest approach (store on state struct temporarily, or pass as param to `reset()`) is left to Claude.
- Whether to add a `WarmStart bool` field to `CommunityResult` for debugging: Claude decides based on plan complexity.

### Deferred Ideas (OUT OF SCOPE)

- Streaming / event-driven graph update pipeline
- Directed graph warm start
- Partial warm start (re-seed only communities near changed nodes)

</user_constraints>

---

## Summary

Warm-start community detection seeds the phase-1 local-move loop with a prior partition instead of the trivial singleton assignment. Because Louvain/Leiden's local-move phase is a greedy hill-climb on modularity Q, starting from a near-optimal partition reduces the number of moves needed before convergence. For small perturbations (≤ 5% edge changes) empirical evidence shows 2–5x fewer passes, translating to proportional speedup.

The key algorithmic insight is that warm start only benefits the **first supergraph level**. Subsequent passes work on a compressed supergraph whose topology is determined by the current partition — those supergraphs are independent of the original graph structure and always start fresh. This is correct and by design (D-02, confirmed by code analysis).

The primary implementation risk is the **commStr recomputation correctness**: after seeding `state.partition` from a prior partition, `state.commStr` must be rebuilt by summing node strengths per community ID from the new graph — not copied from the prior run, since node strengths (weighted degrees) can change when edges are added/removed. Get this wrong and phase-1 ΔQ calculations use stale strength values, silently producing lower-quality partitions.

**Primary recommendation:** Pass `initialPartition map[NodeID]int` as a parameter to `reset()`. This is the cleanest approach — it avoids storing warm-start state on the struct between pool Get and reset, keeps the pool-safe contract explicit, and requires only a one-line change at the two call sites.

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `math/rand` (stdlib) | Go 1.26.1 | RNG for node shuffle | Already used; `rand.New(src)` pattern established |
| `slices` (stdlib) | Go 1.26.1 | Sorting NodeID slices | Already used throughout; no external dependency |
| `sync` (stdlib) | Go 1.26.1 | `sync.Pool` for state reuse | Already used; warm start must remain pool-safe |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `testing` (stdlib) | Go 1.26.1 | Benchmarks and tests | All new benchmarks follow existing `b.ResetTimer()` pattern |

No new external dependencies are needed. This phase is purely internal algorithm changes plus tests.

**Installation:** No new packages required.

---

## Architecture Patterns

### Recommended Project Structure

No new files needed. Changes are confined to:

```
graph/
  detector.go          — add InitialPartition field to LouvainOptions, LeidenOptions
  louvain_state.go     — modify reset() signature; add warm-seed logic
  leiden_state.go      — modify reset() signature; add warm-seed logic
  louvain.go           — update acquireLouvainState() and reset() call sites
  leiden.go            — update acquireLeidenState() and reset() call sites
  benchmark_test.go    — add BenchmarkLouvainWarmStart, BenchmarkLeidenWarmStart
  accuracy_test.go     — add warm-start correctness tests
```

### Pattern 1: reset() Signature Extension

**What:** Add `initialPartition map[NodeID]int` parameter to `reset()`. Nil = cold (existing behavior).

**When to use:** Always — both warm and cold paths go through the same `reset()`, nil-guarded.

```go
// louvain_state.go
func (st *louvainState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
    clear(st.partition)
    clear(st.commStr)
    clear(st.neighborBuf)
    st.neighborDirty = st.neighborDirty[:0]
    st.candidateBuf = st.candidateBuf[:0]

    var src rand.Source
    if seed != 0 {
        src = rand.NewSource(seed)
    } else {
        src = rand.NewSource(time.Now().UnixNano())
    }
    st.rng = rand.New(src)

    nodes := g.Nodes()
    slices.Sort(nodes)

    if initialPartition == nil {
        // Cold start: trivial singleton assignment
        for i, n := range nodes {
            st.partition[n] = i
            st.commStr[i] = g.Strength(n)
        }
        return
    }

    // Warm start: seed from initialPartition
    // Step 1: find max community ID in prior partition to offset new-node singletons
    maxCommID := -1
    for _, c := range initialPartition {
        if c > maxCommID {
            maxCommID = c
        }
    }
    nextNewComm := maxCommID + 1

    // Step 2: assign partition, handling new nodes
    for _, n := range nodes {
        if c, ok := initialPartition[n]; ok {
            st.partition[n] = c
        } else {
            // New node not in prior partition: own singleton community (D-03)
            st.partition[n] = nextNewComm
            nextNewComm++
        }
    }

    // Step 3: compact to 0-indexed contiguous BEFORE computing commStr (D-05)
    // Inline compaction (same logic as normalizePartition but in-place on st.partition)
    remap := make(map[int]int, len(initialPartition))
    next := 0
    for _, n := range nodes { // nodes is already sorted — deterministic remap
        c := st.partition[n]
        if _, exists := remap[c]; !exists {
            remap[c] = next
            next++
        }
        st.partition[n] = remap[c]
    }

    // Step 4: build commStr from new graph strengths (CRITICAL: not from prior run)
    for _, n := range nodes {
        st.commStr[st.partition[n]] += g.Strength(n)
    }
}
```

**Critical note:** `commStr` MUST be built from the new graph's `g.Strength(n)`, not copied from any prior run. Node strengths change when edges are added/removed.

### Pattern 2: acquireLouvainState() Update

**What:** Pass `initialPartition` through from the detector options to `acquireLouvainState` and the first `reset()` call.

**When to use:** In `louvain.go` `Detect()` — warm partition applies only to the very first `reset()` call (on the original graph). Subsequent loop iterations (supergraph passes) always use `nil`.

```go
// louvain.go — Detect() loop
state := acquireLouvainState(currentGraph, seed, d.opts.InitialPartition)
defer releaseLouvainState(state)

for {
    state.reset(currentGraph, seed, nil) // subsequent supergraph passes: cold
    // ...
}
```

**Wait** — this is wrong. The existing code calls `state.reset(currentGraph, seed)` at the TOP of the loop, including the first iteration. So the warm partition must be used on the first iteration only. The cleanest approach:

```go
// louvain.go — Detect()
state := acquireLouvainState(currentGraph, seed)
defer releaseLouvainState(state)

firstPass := true
for {
    if firstPass {
        state.reset(currentGraph, seed, d.opts.InitialPartition)
        firstPass = false
    } else {
        state.reset(currentGraph, seed, nil)
    }
    // ...
}
```

Alternatively (simpler): `acquireLouvainState` returns an already-reset state (the current behavior), but the loop's first `reset()` overwrites it. The real fix is to move the loop's `reset()` call to occur only on iterations > 0, keeping the initial state from `acquireLouvainState`. But that changes existing control flow. The `firstPass` boolean is the safest minimal change.

### Pattern 3: Leiden acquire/reset Update

**What:** Identical pattern to Louvain. `acquireLeidenState` and `leidenState.reset()` get the same `initialPartition` parameter treatment.

**When to use:** In `leiden.go` `Detect()` — same `firstPass` boolean guard.

**Important Leiden nuance (D-07):** `refinedPartition` is left nil (same as cold start). The BFS refinement in `refinePartition()` takes `state.partition` as input and produces `state.refinedPartition` from scratch each iteration regardless. No change needed to `refinePartition`.

### Pattern 4: Benchmark Structure

**What:** Run cold detect, perturb graph, compare warm vs cold on perturbed graph.

```go
// benchmark_test.go
func BenchmarkLouvainWarmStart(b *testing.B) {
    // Setup: cold detect to get prior partition
    det := NewLouvain(LouvainOptions{Seed: 1})
    coldResult, _ := det.Detect(bench10K)

    // Perturb: add/remove ~1% of edges (±100 edges on 10K-node BA graph)
    perturbed := perturbGraph(bench10K, 100, 42)

    // Warm detector uses prior partition
    warmOpts := LouvainOptions{Seed: 1, InitialPartition: coldResult.Partition}
    warmDet := NewLouvain(warmOpts)
    warmDet.Detect(perturbed) // warmup pool

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        warmDet.Detect(perturbed)
    }
}
```

The `perturbGraph` helper needs to be added to `benchmark_test.go` (or `testhelpers_test.go`). It should reproducibly add/remove exactly N edges using a seeded RNG.

### Anti-Patterns to Avoid

- **Copying commStr from prior run:** Stale strength values for changed-degree nodes silently corrupt ΔQ. Always recompute from `g.Strength(n)`.
- **Applying warm seed on supergraph passes:** Supergraph node IDs are freshly assigned integers that do not correspond to original NodeIDs. Applying the prior partition map on a supergraph will silently produce wrong assignments. Always use `nil` after the first pass.
- **Storing initialPartition on the pool object:** The pool may return a state from a previous run that holds a stale `initialPartition` pointer. Pass it through `reset()` parameter only; never store it as a struct field after `reset()` completes.
- **Not compacting community IDs before commStr build:** A gap in community IDs (e.g., IDs 0,1,3,7) causes phase1's `candidateBuf` logic to work correctly (community IDs are just ints), but wastes memory and risks edge cases in `buildSupergraph`. Compact with `normalizePartition`-equivalent inline before `commStr` population.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Community ID compaction after warm seed | Custom remap loop | Inline the same logic as `normalizePartition` (already in `louvain.go`) | Existing function is well-tested; reuse or inline same algorithm |
| Perturbation helper for benchmarks | Ad-hoc edge addition | Seeded `generateBA`-style helper in `benchmark_test.go` | Reproducibility requires seeded RNG; existing `generateBA` is a model |
| Modularity comparison in tests | Custom Q computation | `ComputeModularityWeighted` (already in `modularity.go`) | Already validated, handles resolution parameter correctly |

**Key insight:** All needed primitives already exist. This phase is wiring, not building.

---

## Common Pitfalls

### Pitfall 1: commStr Built from Prior Strengths
**What goes wrong:** `commStr[c]` holds the sum of node strengths for community `c`. If you copy strength values from the prior run rather than recomputing from `g.Strength(n)`, nodes whose degree changed (due to edge additions/removals) will have stale strength values. Phase-1 ΔQ calculations will be wrong, potentially converging to a lower-quality partition than cold start.
**Why it happens:** The natural impulse is to avoid recomputation by copying prior state.
**How to avoid:** Always accumulate `commStr` from `g.Strength(n)` in the current graph, after seeding `partition` from the prior run. See Pattern 1 above.
**Warning signs:** `Q(warm) < Q(cold)` on perturbed graphs — correctness test D-09 catches this.

### Pitfall 2: Warm Seed Applied on Supergraph Passes
**What goes wrong:** The loop in `louvain.go` calls `state.reset(currentGraph, seed)` at the top of every iteration. On passes 2+, `currentGraph` is a supergraph whose node IDs are freshly assigned integers (0, 1, 2, ...). The prior partition uses original `NodeID` values. Applying the warm partition map on a supergraph silently maps those integer IDs to arbitrary community IDs, breaking the algorithm.
**Why it happens:** A naive implementation passes `d.opts.InitialPartition` to every `reset()` call.
**How to avoid:** Use the `firstPass` boolean guard (see Pattern 2). Only the first `reset()` call (when `currentGraph == g`) receives the warm partition.
**Warning signs:** `Detect` returns wildly different Q values across runs with the same seed, or assertion failures in compaction logic.

### Pitfall 3: New Node Singleton Collision
**What goes wrong:** New nodes (in graph, not in prior partition) need a fresh community ID that does not collide with any existing community ID in the seeded partition. If you naively assign them IDs 0, 1, 2, ... you will silently merge them into existing communities.
**Why it happens:** Starting the singleton offset at 0 instead of `max(prior IDs) + 1`.
**How to avoid:** Compute `maxCommID` over `initialPartition` values before assigning new-node singletons (D-03). Compaction in step 3 then normalizes everything to contiguous 0-indexed IDs.
**Warning signs:** Fewer communities returned than expected after warm start on a graph with new nodes.

### Pitfall 4: Import Path Mismatch in New Tests
**What goes wrong:** Existing test files import `community-detection/graph/testdata` — a stale path from before the module rename to `github.com/bluuewhale/loom`. New test code that copies these imports will use the same stale path. This compiles only if the stale path is still resolvable (e.g., via a replace directive or because tests haven't been run post-rename).
**Why it happens:** Module renamed in go.mod but test imports not updated.
**How to avoid:** Match exactly what existing tests use. If existing tests compile and pass, copy their import path verbatim. Do not introduce a mix.
**Warning signs:** `go test ./graph/...` fails with "cannot find module providing package ...".

### Pitfall 5: Pool Safety — initialPartition Lifetime
**What goes wrong:** If `initialPartition` is stored as a field on the pooled `louvainState` struct, the pool may return a state to a different goroutine that still has a stale pointer to the previous caller's partition map. Concurrent Detect calls on different graphs would cross-contaminate each other.
**Why it happens:** Temptation to store it on the struct to avoid passing through `acquireLouvainState`.
**How to avoid:** Pass `initialPartition` only as a parameter to `reset()` and use it only during that call. Never store it as a persistent field on the state struct.
**Warning signs:** Race detector (`go test -race`) fires on `st.partition` or `st.commStr`.

### Pitfall 6: Benchmark Measures the Wrong Thing
**What goes wrong:** If the warm `InitialPartition` is set once at benchmark construction but `Detect` is called `b.N` times in the loop, every iteration after the first runs warm with the same partition — not the result of the previous iteration. This is correct for a controlled benchmark (measuring warm start from a fixed prior). But if the perturbed graph is rebuilt each iteration, alloc counts will dominate.
**Why it happens:** Misplacing graph construction inside vs outside `b.ResetTimer()`.
**How to avoid:** Build the perturbed graph and compute the cold result in setup, before `b.ResetTimer()`. Only the `Detect` call goes inside the loop.

---

## Code Examples

### Warm Seed Logic in reset() — Partition Population

```go
// Source: direct analysis of louvain_state.go reset() + D-02, D-03, D-05 decisions

// Warm path inside reset() after clearing maps and reseeding RNG:
nodes := g.Nodes()
slices.Sort(nodes)

if initialPartition == nil {
    // Cold start — unchanged behavior
    for i, n := range nodes {
        st.partition[n] = i
        st.commStr[i] = g.Strength(n)
    }
    return
}

// Find max community ID in prior partition (needed for D-03 new-node offset)
maxCommID := -1
for _, c := range initialPartition {
    if c > maxCommID {
        maxCommID = c
    }
}
nextNewComm := maxCommID + 1

// Seed partition
for _, n := range nodes {
    if c, ok := initialPartition[n]; ok {
        st.partition[n] = c
    } else {
        st.partition[n] = nextNewComm
        nextNewComm++
    }
}

// Compact community IDs to 0-indexed contiguous (D-05)
// Must use sorted nodes for deterministic remap (mirrors normalizePartition logic)
remap := make(map[int]int)
next := 0
for _, n := range nodes {
    c := st.partition[n]
    if _, ok := remap[c]; !ok {
        remap[c] = next
        next++
    }
    st.partition[n] = remap[c]
}

// Rebuild commStr from current graph strengths (CRITICAL — not from prior run)
for _, n := range nodes {
    st.commStr[st.partition[n]] += g.Strength(n)
}
```

### firstPass Guard in Detect()

```go
// Source: analysis of louvain.go Detect() loop structure

state := acquireLouvainState(currentGraph, seed)
defer releaseLouvainState(state)

firstPass := true
for {
    if firstPass {
        state.reset(currentGraph, seed, d.opts.InitialPartition)
        firstPass = false
    } else {
        state.reset(currentGraph, seed, nil)
    }
    moves := phase1(currentGraph, state, resolution, currentGraph.TotalWeight())
    // ... rest of loop unchanged
}
```

### perturbGraph Helper for Benchmarks

```go
// To be added in benchmark_test.go or testhelpers_test.go

// perturbGraph returns a copy of g with n edges added and n edges removed,
// using a seeded RNG for reproducibility.
func perturbGraph(g *Graph, n int, seed int64) *Graph {
    rng := rand.New(rand.NewSource(seed))
    nodes := g.Nodes()

    // Deep copy: add all existing edges
    pg := NewGraph(false)
    for _, e := range g.Edges() {
        pg.AddEdge(e.From, e.To, e.Weight)
    }

    // Remove n random existing edges
    edges := pg.Edges()
    rng.Shuffle(len(edges), func(i, j int) { edges[i], edges[j] = edges[j], edges[i] })
    for i := 0; i < n && i < len(edges); i++ {
        pg.RemoveEdge(edges[i].From, edges[i].To)
    }

    // Add n random new edges (between existing nodes, skip self-loops)
    added := 0
    for added < n {
        a := nodes[rng.Intn(len(nodes))]
        b := nodes[rng.Intn(len(nodes))]
        if a != b {
            pg.AddEdge(a, b, 1.0)
            added++
        }
    }
    return pg
}
```

**Note:** This requires that `Graph` exposes `Edges()` returning a slice and `RemoveEdge()`. Verify these methods exist in `graph.go` before using. If `RemoveEdge` does not exist, use a rebuild approach: copy only edges not in the removal set.

### Warm Start Correctness Test Pattern

```go
// accuracy_test.go addition — verifying D-09

func TestLouvainWarmStartQuality(t *testing.T) {
    g := buildGraph(testdata.KarateClubEdges)
    det := NewLouvain(LouvainOptions{Seed: 1})

    // Cold run
    coldResult, _ := det.Detect(g)

    // Perturb graph slightly
    perturbed := perturbGraph(g, 2, 99) // 2 edge changes on 78-edge graph (~2.5%)

    // Cold run on perturbed
    coldPerturbed, _ := det.Detect(perturbed)

    // Warm run on perturbed using cold result as seed
    warmDet := NewLouvain(LouvainOptions{Seed: 1, InitialPartition: coldResult.Partition})
    warmResult, _ := warmDet.Detect(perturbed)

    // Q(warm) should be >= Q(cold) on perturbed graph
    if warmResult.Modularity < coldPerturbed.Modularity-1e-9 {
        t.Errorf("warm Q=%.4f < cold Q=%.4f — warm start degraded quality",
            warmResult.Modularity, coldPerturbed.Modularity)
    }
    t.Logf("cold Q=%.4f passes=%d, warm Q=%.4f passes=%d",
        coldPerturbed.Modularity, coldPerturbed.Passes,
        warmResult.Modularity, warmResult.Passes)
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Always cold start (singleton init) | Warm start from prior partition | Phase 05 | Faster convergence on incremental graph updates |
| Phase1 iterates to convergence from scratch | Phase1 begins near optimum; fewer moves needed | Phase 05 | 2–5x fewer passes expected for ≤5% edge perturbation |

**Algorithmic background (established literature, HIGH confidence):**
- Louvain (Blondel et al. 2008) is a greedy modularity optimization. Warm starting is standard practice — the algorithm will converge to the same or better local optimum when initialized near the optimum, because the greedy moves can only increase Q.
- Leiden (Traag et al. 2019) adds BFS refinement to fix disconnected communities. Warm start applies identically to the local-move phase (phase 1). The BFS refinement (phase 2) is a correction step that always runs fresh on the current `state.partition`, so warm start does not interfere with Leiden's connectivity guarantees.
- Neither algorithm guarantees global optimality (NP-hard). Warm start does not increase the risk of a worse result compared to cold start — at worst, convergence is equally fast if the graph changed significantly enough that the prior partition is far from the new optimum.
- **Documented risk:** For large perturbations (>20% edge changes), warm start may converge to a local optimum different from cold start, with similar or marginally lower Q. This is acceptable; the phase design (D-09) tests only small perturbations.

---

## Open Questions

1. **Graph API: does `RemoveEdge` exist?**
   - What we know: `graph.go` was not read in detail; `Edges()` returning a slice is used in `buildSupergraph` (via `Neighbors`) but a `RemoveEdge` method is not referenced anywhere in the read files.
   - What's unclear: Whether `Graph` supports edge removal or is append-only.
   - Recommendation: Planner should read `graph.go` before writing the perturbGraph helper. If `RemoveEdge` does not exist, use the rebuild-without-removed-edges approach: copy edges selectively, skipping N random ones.

2. **Import path: `community-detection/graph/testdata` vs `github.com/bluuewhale/loom/graph/testdata`**
   - What we know: All existing tests use `community-detection/graph/testdata`. The go.mod module is `github.com/bluuewhale/loom`. The tests presumably still pass (v1.0 shipped).
   - What's unclear: Whether there is a `replace` directive or workspace file that makes the stale path resolve, or whether the tests are actually broken post-rename.
   - Recommendation: New test code should copy the exact import path used by existing passing tests. The planner should check whether `go test ./graph/...` passes before adding new tests and use whatever path compiles.

3. **`newLouvainState` signature update**
   - What we know: `newLouvainState(g *Graph, seed int64)` in `louvain_state.go` is used by `leiden.go`'s inline wrapper pattern (`&louvainState{...}` is constructed inline, not via `newLouvainState`). The `newLouvainState` function comment says "kept for backward-compatibility".
   - What's unclear: Whether any code outside the read files calls `newLouvainState`.
   - Recommendation: `newLouvainState` does not need a warm-start parameter since Leiden constructs the `louvainState` inline. But if the planner wishes to keep it consistent, add `initialPartition map[NodeID]int` with nil = cold. It is low priority.

4. **`WarmStart bool` field on `CommunityResult` (Claude's discretion)**
   - Recommendation: **Skip it.** The field adds noise to the struct for a minor debugging convenience. Callers can infer warm start was used because they set `InitialPartition` themselves. Adding the field means it must be set in both Louvain and Leiden `Detect()` paths, adding two more touch points for no algorithmic benefit.

---

## Environment Availability

Step 2.6: SKIPPED — phase is purely internal code changes in an existing Go module. No external tools, services, databases, or CLIs beyond the Go toolchain are required. Go toolchain availability is a project-wide prerequisite already validated at v1.0.

---

## Validation Architecture

`nyquist_validation` is explicitly `false` in `.planning/config.json`. Section skipped.

---

## Sources

### Primary (HIGH confidence)
- Direct read of `graph/louvain.go` — Detect() loop structure, phase1, buildSupergraph, normalizePartition
- Direct read of `graph/louvain_state.go` — reset(), acquireLouvainState(), pool pattern, RNG seeding
- Direct read of `graph/leiden.go` — Detect() loop, refinePartition(), refinedPartition usage
- Direct read of `graph/leiden_state.go` — reset(), leidenState fields, pool pattern
- Direct read of `graph/detector.go` — LouvainOptions, LeidenOptions, CommunityResult
- Direct read of `graph/benchmark_test.go` — generateBA, BenchmarkLouvain10K pattern
- Direct read of `graph/accuracy_test.go` — NMI test patterns, fixture usage
- Direct read of `graph/testhelpers_test.go` — nmi(), buildGraph(), groundTruthPartition()
- Direct read of `.planning/phases/05-warm-start/05-CONTEXT.md` — all locked decisions D-01..D-09

### Secondary (MEDIUM confidence)
- Blondel, V.D. et al. (2008). "Fast unfolding of communities in large networks." _Journal of Statistical Mechanics_. — Louvain algorithm; warm-start behavior follows from greedy Q-maximization property.
- Traag, V.A. et al. (2019). "From Louvain to Leiden: guaranteeing well-connected communities." _Scientific Reports_. — Leiden BFS refinement; warm start does not interact with refinement correctness.

---

## Metadata

**Confidence breakdown:**
- API changes (D-01): HIGH — trivial struct field addition, zero-value-safe pattern already established
- reset() warm-seed logic (D-02..D-05): HIGH — code fully read; logic is mechanical population of existing maps
- firstPass guard (loop interaction): HIGH — read the actual loop; confirmed reset() called at top of every iteration
- commStr correctness risk: HIGH — critical finding from code analysis; directly verified
- Benchmark structure (D-08): HIGH — existing benchmark pattern is clear and directly reusable
- Correctness test structure (D-09): HIGH — existing accuracy test pattern is directly reusable
- Algorithmic quality guarantee (warm ≥ cold Q): MEDIUM — established from Louvain/Leiden literature; not formally proven in this codebase context but consistent with greedy hill-climb theory

**Research date:** 2026-03-30
**Valid until:** 2026-06-30 (stable domain; only invalidated if core algorithm files change significantly)
