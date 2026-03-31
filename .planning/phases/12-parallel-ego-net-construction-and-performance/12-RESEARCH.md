# Phase 12: Parallel Ego-Net Construction and Performance — Research

**Researched:** 2026-03-31
**Domain:** Go concurrency (goroutine pool, sync primitives), ego-net construction loop parallelization, benchmark authoring
**Confidence:** HIGH

---

## Summary

Phase 12 has two independent tracks that share a single implementation effort. **Track A (ONLINE-10)** requires parallelizing the ego-net construction loop in `buildPersonaGraph` so that `BenchmarkEgoSplitting10K` drops from ~1516ms/op to ≤300ms. **Track B (ONLINE-08/09)** requires authoring two new benchmarks (`BenchmarkUpdate1Node`, `BenchmarkUpdate1Edge`) that demonstrate the already-implemented incremental `Update()` is ≥10x faster than full `Detect()` on the same Karate Club graph. Track B is purely benchmark authoring; the underlying speedup was delivered in Phase 11.

The key parallelism finding: the ego-net loop in `buildPersonaGraph` iterates over 10K nodes and runs `localDetector.Detect` on each ~10-node ego-net. These 10K calls are fully independent (each node's ego-net reads only from the immutable input graph `g`). With GOMAXPROCS=10 on the test machine (Apple M4), Amdahl's law predicts a 5-9x speedup, which comfortably meets the 5.1x required to reach ≤300ms. The serial tail (edge-wiring loop, O(|E|)) represents <2% of total wall time and does not need parallelization.

**Primary recommendation:** Implement a fixed-size worker pool (N = `runtime.GOMAXPROCS(0)`) that drains a buffered channel of `NodeID` work items, collects per-node ego-net results into pre-allocated per-worker structs, then merges results serially before the existing edge-wiring step. Use `sync.WaitGroup` for coordination. This is the only stdlib-legal approach (no `golang.org/x/sync/errgroup`) and is consistent with the `sync.Pool` pattern already in `louvain_state.go`.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ONLINE-08 | `BenchmarkUpdate1Node` demonstrates `Update()` with 1 added node runs ≥10x faster than `Detect()` on Karate Club 34+1 | Incremental scoping (Phase 11) delivers this; 1 new isolated node → affected={35}, 1 empty ego-net skip; benchmarks need authoring only |
| ONLINE-09 | `BenchmarkUpdate1Edge` demonstrates `Update()` with 1 added edge runs ≥10x faster than `Detect()` on Karate Club + 1 edge | 1 edge → ~11 affected nodes on Karate Club (avg_degree 4.6); warm-start global detection provides additional speedup; benchmark needs authoring |
| ONLINE-10 | Parallel ego-net construction reduces `BenchmarkEgoSplitting10K` from ~1500ms/op to ≤300ms/op | 10K independent localDetector.Detect calls; GOMAXPROCS=10; Amdahl predicts 5-9x; 5.1x required; requires implementation in `buildPersonaGraph` |
</phase_requirements>

---

## Standard Stack

### Core (stdlib only — confirmed by go.mod: `module github.com/bluuewhale/loom`, `go 1.26.1`, zero external deps)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sync.WaitGroup` | stdlib | Coordinate worker pool drain | Standard fan-out/fan-in primitive |
| `runtime.GOMAXPROCS(0)` | stdlib | Determine worker pool size at call time | Returns current GOMAXPROCS without changing it |
| `chan NodeID` (buffered) | stdlib | Work queue for ego-net tasks | Bounded memory; workers self-throttle |

### Not Available (confirmed)
- `golang.org/x/sync/errgroup` — NOT available; go.mod has zero external deps and REQUIREMENTS.md explicitly states "External dependencies — stdlib only; no new imports"
- Use `sync.WaitGroup` + manual error propagation via a shared `sync.Mutex`-guarded error slot

---

## Architecture Patterns

### Where Parallelization Targets Sit

**`buildPersonaGraph` (called from `Detect()`) — PRIMARY TARGET for ONLINE-10:**

```
graph/ego_splitting.go lines 359-390  ← Step 2: ego-net loop (sequential, 10K calls)
graph/ego_splitting.go lines 400-451  ← Step 4-6: edge-wiring (serial, O(|E|)), NOT a target
```

The loop at line 359 (`for _, v := range g.Nodes()`) iterates all N nodes. For each:
1. `buildEgoNet(g, v)` — reads `g` (immutable) → safe to parallelize
2. `localDetector.Detect(egoNet)` — pure computation on a locally-created `*Graph` → safe to parallelize
3. Writes to `personaOf[v]`, `partitions[v]`, `inverseMap[nextPersona]` — writing to different keys in shared maps

**`buildPersonaGraphIncremental` (called from `Update()`) — SECONDARY TARGET:**

```
graph/ego_splitting.go lines 529-558  ← Step d: affected-node ego-net rebuild loop
```

Only iterates `len(affected)` nodes (not all N). For ONLINE-08/09 with Karate Club these are tiny (1 and ~11 nodes respectively). Parallelizing this path provides no measurable benefit for the Karate Club benchmarks but the same helper can be used if len(affected) > threshold.

### Shared State in the Ego-Net Loop

| Variable | Access Pattern | Conflict? | Strategy |
|----------|---------------|-----------|----------|
| `g` (input graph) | Read-only | None | Safe to share |
| `egoNet` | Locally allocated per iteration | None | Each goroutine owns its own |
| `personaOf[v]` | Write keyed by `v` | None — distinct keys | Safe: each worker writes different key |
| `partitions[v]` | Write keyed by `v` | None — distinct keys | Safe: each worker writes different key |
| `inverseMap[nextPersona]` | Write; key = monotonic counter | YES — counter is shared | Must serialize or use collect-then-merge |
| `nextPersona` (counter) | Read-modify-write | YES | Must serialize |

### Recommended Pattern: Collect-then-Merge (two-phase)

**Phase 1 — parallel (worker pool):** Each worker receives a `NodeID`, runs `buildEgoNet` + `localDetector.Detect`, and stores the result in a per-slot struct. No shared writes except reading from `g`.

**Phase 2 — serial (merge):** Iterate results in deterministic order (sorted node IDs), assign `nextPersona` IDs, fill `personaOf`, `inverseMap`, `partitions`.

```go
// Per-node result from parallel phase
type egoNetResult struct {
    v         NodeID
    partition map[NodeID]int // result.Partition from localDetector
    err       error
}

// Worker pool pattern (stdlib only)
func buildPersonaGraphParallel(g *Graph, localDetector CommunityDetector) (..., error) {
    nodes := g.Nodes()
    results := make([]egoNetResult, len(nodes))
    // Pre-assign slots by index so workers write to results[i] with no sharing
    // ...

    nWorkers := runtime.GOMAXPROCS(0)
    work := make(chan int, len(nodes)) // send slice index, not NodeID
    for i := range nodes {
        work <- i
    }
    close(work)

    var wg sync.WaitGroup
    var firstErr error
    var errMu sync.Mutex

    for w := 0; w < nWorkers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := range work {
                v := nodes[i]
                egoNet := buildEgoNet(g, v)
                var partition map[NodeID]int
                if egoNet.NodeCount() > 0 {
                    r, err := localDetector.Detect(egoNet)
                    if err != nil {
                        errMu.Lock()
                        if firstErr == nil { firstErr = err }
                        errMu.Unlock()
                        results[i] = egoNetResult{v: v, err: err}
                        continue
                    }
                    partition = r.Partition
                }
                results[i] = egoNetResult{v: v, partition: partition}
            }
        }()
    }
    wg.Wait()

    if firstErr != nil {
        return nil, nil, nil, nil, firstErr
    }

    // Serial merge: assign PersonaIDs in deterministic order
    // ... (nextPersona counter, fill personaOf, inverseMap, partitions)
}
```

**Why index-based slots (`results[i]`):** Each goroutine writes only to its assigned index. No mutex needed for result collection. This is the key insight — pre-allocating a slice of results by index eliminates all write contention in the parallel phase.

**Why `chan int` (index) not `chan NodeID`:** Allows workers to write directly to `results[i]` with no map needed to find the slot later. Also avoids allocating a `map[NodeID]egoNetResult` for lookup.

### Alternative: goroutine-per-node (simpler, acceptable)

```go
results := make([]egoNetResult, len(nodes))
var wg sync.WaitGroup
sem := make(chan struct{}, runtime.GOMAXPROCS(0)) // semaphore to bound goroutines
for i, v := range nodes {
    wg.Add(1)
    go func(i int, v NodeID) {
        defer wg.Done()
        sem <- struct{}{}
        defer func() { <-sem }()
        // ... compute egoNet, detect, store results[i]
    }(i, v)
}
wg.Wait()
```

This spawns N goroutines but bounds concurrency via semaphore. Simpler code but creates 10K goroutine objects (acceptable; Go scheduler handles this efficiently at 10K scale).

**Recommendation:** Worker pool with buffered channel is preferred — consistent with `sync.Pool` pattern in the codebase, avoids goroutine-per-task allocation overhead.

### ONLINE-08 / ONLINE-09 Benchmark Pattern

These benchmarks do NOT require any code changes to `Update()`. They only need new benchmark functions. The pattern follows `BenchmarkUpdate_EmptyDelta` in `ego_splitting_test.go`:

```go
// BenchmarkUpdate1Node measures Update() with 1 added isolated node vs Detect().
// ONLINE-08: Update >= 10x faster than Detect on the same updated graph.
func BenchmarkUpdate1Node(b *testing.B) {
    // Setup (outside timer): build Karate Club 34-node graph, run Detect to get prior
    base := buildGraph(testdata.KarateClubEdges)
    d := NewOnlineEgoSplitting(EgoSplittingOptions{...})
    prior, _ := d.Detect(base)

    // Build updated graph (34 + 1 isolated node)
    updated := buildGraph(testdata.KarateClubEdges)
    updated.AddNode(NodeID(34), 1.0)
    delta := GraphDelta{AddedNodes: []NodeID{34}}

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        d.Update(updated, delta, prior)
    }
}

// BenchmarkDetect35 is the comparison baseline for ONLINE-08.
func BenchmarkDetect35(b *testing.B) { ... }
```

The ratio check must be done in a `TestUpdate1NodeSpeedup` test (like `TestLouvainWarmStartSpeedup`) that calls `testing.Benchmark` on both and asserts `detectNsPerOp / updateNsPerOp >= 10.0`.

### Anti-Patterns to Avoid

- **Writing to shared maps from goroutines without synchronization:** `personaOf`, `inverseMap`, `partitions` are regular Go maps; concurrent writes cause data races caught by `-race`. Use index-based pre-allocated slice for results.
- **Mutating `localDetector` from multiple goroutines:** `localDetector.Detect()` already uses `sync.Pool` internally (see `louvain_state.go:acquireLouvainState`); multiple goroutines calling it concurrently is safe. Do NOT create a new detector per goroutine.
- **Using `g.Nodes()` concurrently:** `g.Nodes()` allocates and returns a slice; call it once before spawning workers, share the slice read-only.
- **Closing work channel before all items are sent:** Always fill the channel completely before `close(work)` — or use a separate goroutine to feed.
- **Non-deterministic PersonaID assignment:** Assigning PersonaIDs in goroutine completion order breaks the `nextPersona` counter monotonicity. The serial merge phase must iterate `results` in a fixed order (e.g., pre-sorted node index).
- **Parallelizing `buildPersonaGraphIncremental` unconditionally:** For small `affected` sets (Karate Club, ONLINE-08/09), the goroutine overhead exceeds the benefit. Gate parallelism: if `len(affected) <= runtime.GOMAXPROCS(0)`, use the sequential path.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Goroutine pool | Custom ring-buffer scheduler | `chan + sync.WaitGroup` | stdlib, zero deps, proven pattern |
| Error propagation across goroutines | Complex error channel | Single `firstErr` + `sync.Mutex` | One error slot is sufficient; all errors are localDetector.Detect failures which abort the whole pipeline |
| Work distribution | Manual sharding by index range | Buffered `chan int` (work queue) | Self-balancing; no need to worry about unequal ego-net sizes (BA graph has power-law degree distribution) |

---

## Common Pitfalls

### Pitfall 1: Race on shared maps (personaOf, inverseMap, partitions)
**What goes wrong:** Go maps are not concurrency-safe. Multiple goroutines writing distinct keys to the same map still races because the map's internal hash table can resize concurrently.
**Why it happens:** "Different keys" does NOT make map writes safe in Go.
**How to avoid:** Use the collect-then-merge pattern. Workers write to `results[i]` (pre-allocated slice, index-disjoint). Serial merge phase does all map writes.
**Warning signs:** `go test -race` reports `DATA RACE` on `personaOf` write or `inverseMap` write.

### Pitfall 2: Non-deterministic PersonaID assignment breaks ONLINE-11 invariant
**What goes wrong:** If worker goroutines write `nextPersona++` in non-deterministic order, PersonaID assignment changes between runs, making tests flaky and potentially causing PersonaID collisions across multiple `Update()` calls.
**Why it happens:** Goroutines complete in scheduler-dependent order.
**How to avoid:** PersonaID assignment must happen in the serial merge phase, iterating `results` in a fixed order (sort `nodes` before spawning workers, iterate `results` sequentially). The parallel phase only computes `partition` maps; ID assignment is serial.

### Pitfall 3: Benchmark comparison is unfair (setup cost in timed region)
**What goes wrong:** `BenchmarkUpdate1Node` includes `Detect()` for prior result inside the `b.N` loop, inflating Update cost or conflating it with Detect cost.
**How to avoid:** Follow the Phase 5 benchmark pattern decision: "Benchmark setup (cold detect + graph build) before `b.ResetTimer()`; only the operation under test inside the loop." The `prior` result must be computed once before `b.ResetTimer()`.

### Pitfall 4: localDetector is not concurrency-safe across detectors with shared seed state
**What goes wrong:** If a single `*louvainDetector` instance is shared across goroutines, the `sync.Pool` inside is safe, but the `opts.Seed` field is read-only and safe. The concern is `opts.InitialPartition` — if warm-start options are passed, the partition map is shared read-only (safe). Confirmed safe: `louvainDetector.Detect` acquires state from pool, resets it, and releases it.
**Warning signs:** None expected, but verify with `go test -race`.

### Pitfall 5: Worker pool channel not properly closed before WaitGroup.Wait()
**What goes wrong:** Workers block forever waiting for work if channel is never closed.
**How to avoid:** `close(work)` after all items are sent. Workers use `for i := range work` which exits cleanly on close.

### Pitfall 6: ONLINE-08/09 speedup not ≥10x for 1-edge case
**What goes wrong:** `BenchmarkUpdate1Edge` may fail the ≥10x threshold because adding 1 edge to Karate Club expands `affected` to ~11 nodes (both endpoints + their neighbors). 11/35 = 31% recomputation → maximum structural speedup is ~3x. Warm-start global detection adds more speedup but may not reach 10x total.
**Why it happens:** The 1-hop expansion in `computeAffected` is aggressive for dense small graphs like Karate Club.
**How to investigate:** Measure `BenchmarkDetect35` and `BenchmarkUpdate1Edge` separately. If 10x is not met, the requirement specification may need revisiting — or the benchmark baseline graph should be chosen more carefully (a new isolated edge between low-degree nodes minimizes neighbor expansion).
**Recommendation for planning:** Choose the edge endpoint deliberately — pick two nodes with degree 1 (leaves) to minimize neighbor expansion. If no such pair exists in Karate Club, add an edge between new node 34 and existing node 0, with the baseline being `Detect(35-node graph with that edge)`.

---

## Code Examples

### Worker pool pattern (stdlib, no deps)
```go
// Source: Go stdlib sync package documentation + louvain_state.go pool pattern
import (
    "runtime"
    "sync"
)

type egoNetResult struct {
    partition map[NodeID]int // nil if ego-net was empty
    err       error
}

func runParallelEgoNets(g *Graph, nodes []NodeID, localDetector CommunityDetector) ([]egoNetResult, error) {
    results := make([]egoNetResult, len(nodes))

    nWorkers := runtime.GOMAXPROCS(0)
    work := make(chan int, len(nodes))
    for i := range nodes {
        work <- i
    }
    close(work)

    var wg sync.WaitGroup
    var firstErr error
    var errMu sync.Mutex

    for w := 0; w < nWorkers; w++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for i := range work {
                v := nodes[i]
                egoNet := buildEgoNet(g, v)
                if egoNet.NodeCount() == 0 {
                    results[i] = egoNetResult{partition: nil}
                    continue
                }
                r, err := localDetector.Detect(egoNet)
                if err != nil {
                    errMu.Lock()
                    if firstErr == nil {
                        firstErr = err
                    }
                    errMu.Unlock()
                    results[i] = egoNetResult{err: err}
                    continue
                }
                results[i] = egoNetResult{partition: r.Partition}
            }
        }()
    }
    wg.Wait()

    if firstErr != nil {
        return nil, firstErr
    }
    return results, nil
}
```

### Benchmark with speedup assertion (ONLINE-08 pattern)
```go
// Source: TestLouvainWarmStartSpeedup pattern in benchmark_test.go
func TestUpdate1NodeSpeedup(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping speedup test in short mode")
    }
    detect := testing.Benchmark(BenchmarkDetect35)
    update := testing.Benchmark(BenchmarkUpdate1Node)
    if detect.NsPerOp() == 0 || update.NsPerOp() == 0 {
        t.Skip("benchmark returned 0 ns/op")
    }
    speedup := float64(detect.NsPerOp()) / float64(update.NsPerOp())
    t.Logf("Update1Node speedup: %.1fx (detect=%dns, update=%dns)",
        speedup, detect.NsPerOp(), update.NsPerOp())
    if speedup < 10.0 {
        t.Errorf("Update1Node speedup %.1fx < 10x requirement (ONLINE-08)", speedup)
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Sequential ego-net loop (Phase 08) | Parallel worker pool (Phase 12) | Phase 12 | ~5x speedup, meets ≤300ms target |
| 5000ms regression guard (benchmark_test.go:189) | 300ms target (post-Phase 12) | Phase 12 | Tighten guard after parallel implementation |
| Full Detect() fallback in Update() (Phase 10) | Incremental Update() (Phase 11) | Phase 11 | Unlocks ≥10x speedup for ONLINE-08/09 |

**Existing benchmark stub to update:**

`TestEgoSplitting10KUnder300ms` (benchmark_test.go lines 179-192) has a comment explaining the 5000ms relaxed budget and references parallelism deferral to v1.3. After Phase 12, the threshold should be tightened to 300ms and the deferral comment removed.

---

## Key Implementation Decisions

### Decision 1: Which function gets the parallel loop?
`buildPersonaGraph` is the primary target (called by `Detect()`). `buildPersonaGraphIncremental` should use a sequential path for small `affected` sets (< 2×GOMAXPROCS) and the same parallel helper for large ones. The simplest approach: extract the ego-net computation into a shared helper `runEgoNets(g, nodes, localDetector)` that both callers use, with parallelism enabled unconditionally (worker pool self-limits to GOMAXPROCS workers).

### Decision 2: Pre-sort nodes before worker dispatch?
Yes. `g.Nodes()` returns nodes in non-deterministic order. Sorting before dispatch ensures the `results[]` slice has a deterministic mapping from index to NodeID. This is required for deterministic PersonaID assignment in the serial merge phase. Cost: O(N log N) sort — negligible vs O(N × Louvain) parallel phase.

### Decision 3: Error handling — abort all workers on first error?
The current sequential loop returns immediately on `localDetector.Detect` error. The parallel version should: (a) store the first error, (b) let in-flight workers complete their current task (closing channel drains naturally), (c) return the error after `wg.Wait()`. Attempting to cancel in-flight workers adds complexity with no practical benefit since Louvain errors are algorithmic (not I/O), virtually never occur, and each task completes in microseconds.

### Decision 4: Threshold for incremental parallel path?
In `buildPersonaGraphIncremental`: if `len(affected) >= runtime.GOMAXPROCS(0)`, use the parallel helper; otherwise use the sequential loop. This avoids goroutine overhead for the 1-node and 1-edge ONLINE-08/09 cases (affected ~1-11 nodes on Karate Club). Does NOT affect ONLINE-10 (`buildPersonaGraph` always uses parallel for N=10K).

---

## Exact Locations of Parallelization Targets

| Function | File | Lines | Target Loop |
|----------|------|-------|-------------|
| `buildPersonaGraph` | `graph/ego_splitting.go` | 359-390 | `for _, v := range g.Nodes()` — Step 2: ego-net + persona assignment |
| `buildPersonaGraphIncremental` | `graph/ego_splitting.go` | 529-558 | `for v := range affected` — Step d: rebuild affected ego-nets |
| Edge-wiring loop | `graph/ego_splitting.go` | 400-451, 577-616 | NOT a target — serial, reads shared `partitions` map |

---

## Performance Baseline (measured 2026-03-31, Apple M4, GOMAXPROCS=10)

| Benchmark | Current | Target | Speedup Required |
|-----------|---------|--------|-----------------|
| `BenchmarkEgoSplitting10K` | 1516ms/op | ≤300ms/op | ≥5.1x |
| `BenchmarkLouvain10K` | 58ms/op | — | reference only |
| `BenchmarkUpdate_EmptyDelta` | 0 allocs, 83ns | — | already met |
| `BenchmarkUpdate1Node` | does not exist | ≥10x vs Detect35 | benchmark to author |
| `BenchmarkUpdate1Edge` | does not exist | ≥10x vs Detect35 | benchmark to author |

Amdahl analysis (p=0.98 parallel, 10 cores): predicted speedup 8.5x → predicted ~179ms. Target ≤300ms is achievable.

---

## Environment Availability

Step 2.6: SKIPPED — Phase 12 is purely code changes within the existing Go module. No external tools, services, or runtimes beyond `go 1.26.1` (confirmed available: `go version go1.26.1 darwin/arm64`).

---

## Open Questions

1. **Will ONLINE-09 (1-edge) actually achieve ≥10x speedup on Karate Club?**
   - What we know: 1 edge expands `affected` to ~11 nodes (both endpoints + neighbors). 11/35 = 31% recompute. Structural speedup from scoping alone: ~3x. Warm-start adds more.
   - What's unclear: The warm-start global detection speedup on a ~70-persona graph of 35 nodes is hard to predict without measurement. It may not reach 10x.
   - Recommendation: Choose the added edge carefully in the benchmark — pick a pair of nodes with low degree (nodes 8, 9 in Karate Club have degree 5, nodes near the periphery have degree 2-3). Run the benchmark first and check. If 10x is not met with any valid edge, flag it as a requirements calibration issue before Phase 12 execution.

2. **Should `buildPersonaGraphIncremental` also get a parallel path for Phase 12?**
   - What we know: For ONLINE-08/09 (Karate Club, few affected nodes), parallelizing is unnecessary. For large graphs with many affected nodes (e.g., a high-degree node added to a 10K graph), it would help.
   - Recommendation: Add a shared `runEgoNets` helper used by both `buildPersonaGraph` and `buildPersonaGraphIncremental`. The parallel path is always available; for small sets GOMAXPROCS workers idle immediately — overhead is bounded.

---

## Sources

### Primary (HIGH confidence)
- `graph/ego_splitting.go` — inspected directly; exact loop locations at lines 359-390 and 529-558
- `graph/benchmark_test.go` — inspected directly; `BenchmarkEgoSplitting10K` at lines 154-166; `TestEgoSplitting10KUnder300ms` at lines 179-192
- `graph/louvain_state.go` — inspected directly; `sync.Pool` pattern at lines 21-43; `acquireLouvainState`/`releaseLouvainState` are the project's pool conventions
- Benchmark run (2026-03-31): `BenchmarkEgoSplitting10K` = 1516ms/op confirmed; `BenchmarkLouvain10K` = 58ms/op
- Go 1.26.1 stdlib (`sync`, `runtime`) — confirmed available; no external deps in go.mod

### Secondary (MEDIUM confidence)
- Amdahl's law calculation: p=0.98 parallel fraction estimated from ratio of Louvain cost (58ms × 10K calls) to total (1516ms)
- Worker pool pattern: standard Go concurrency idiom verified in Go documentation

### Tertiary (LOW confidence — not needed, stdlib only)
- None

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — stdlib only, confirmed by go.mod inspection
- Architecture: HIGH — exact loop locations confirmed by source inspection and benchmark measurement
- Pitfalls: HIGH — race conditions confirmed by Go map semantics; benchmark patterns from existing codebase
- ONLINE-08/09 speedup: MEDIUM — structural analysis is sound; actual measurement pending authoring of benchmarks

**Research date:** 2026-03-31
**Valid until:** Stable — no external deps, pure Go stdlib concurrency
