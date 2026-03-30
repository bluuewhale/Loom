# Phase 05: Warm Start — Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Add warm-start (incremental / online community detection) to both Louvain and Leiden:
when the caller supplies a prior `CommunityResult.Partition`, the algorithm seeds its
initial state from that partition instead of the trivial singleton partition, converging
faster when the graph has changed only slightly.

This phase covers:
- API extension to accept an initial partition
- `reset()` logic changes in `louvain_state.go` and `leiden_state.go`
- Correctness tests (warm start produces equal or better Q vs cold start on small perturbations)
- Benchmark comparison (warm vs cold on 10K-node graph with small edge changes)

Out of scope for this phase:
- Streaming / continuous graph update pipelines
- Directed graph support (still deferred to v2)
- Automatic change-detection heuristics

</domain>

<decisions>
## Implementation Decisions

### API Surface
- **D-01:** Extend `LouvainOptions` and `LeidenOptions` with `InitialPartition map[NodeID]int`.
  Nil (zero value) = cold start — existing behaviour is fully preserved with no breaking change.
  Callers pass `prior.Partition` directly without any wrapper constructor.
  Rationale: fits the established zero-value-safe options pattern; no new interfaces or constructors needed.

### Initial State Injection Point
- **D-02:** Warm start is applied inside `reset()` in `louvain_state.go` and `leiden_state.go`.
  When `opts.InitialPartition != nil`, populate `state.partition` from it instead of the
  trivial `i → i` singleton assignment.
  `commStr` is then recomputed from the seeded partition (same logic as cold path, just over
  a non-trivial initial partition).
  This requires `reset()` to receive the options struct (or just the `InitialPartition` map).

### New Node Handling (node in graph but not in prior partition)
- **D-03:** Assign to a fresh singleton community (own ID, starting after `max(prior community IDs) + 1`).
  Phase 1 local moves will naturally absorb these nodes into the best-gain community.
  No error is returned.

### Removed Node Handling (node in prior partition but not in graph)
- **D-04:** Silently ignored — the reset loop only iterates `g.Nodes()`, so stale keys in
  `InitialPartition` are simply never read.

### Community ID Compaction
- **D-05:** After seeding from `InitialPartition`, re-compact community IDs to 0-indexed
  contiguous using the same `normalizePartition` path that already exists post-convergence.
  This keeps internal invariants intact.
  *(If `normalizePartition` is not already exposed internally, inline the compaction in `reset()`.)*

### Convergence Criteria
- **D-06:** No change. Same `Tolerance`, `MaxPasses`/`MaxIterations` apply whether warm or cold.
  The caller controls convergence budget; warm start simply provides a better starting point.

### Leiden `refinedPartition` Seeding
- **D-07:** On warm start, `leidenState.refinedPartition` is left nil (same as cold start).
  It is populated during the first BFS refinement pass as usual.
  Rationale: seeding the refined partition would couple warm-start logic to Leiden internals
  with unclear benefit; the warm benefit comes from the local-move phase starting near the
  optimum, not from skipping refinement.

### Benchmarks
- **D-08:** Add `BenchmarkLouvainWarmStart` and `BenchmarkLeidenWarmStart` in `benchmark_test.go`.
  Test scenario: run cold Detect on a 10K-node graph, then add/remove ~1% of edges
  (±100 edges), and compare warm re-detect time vs cold re-detect time.
  The benchmark must show a measurable speedup (target: warm ≤ 50% of cold ns/op for small perturbations).

### Correctness Tests
- **D-09:** Add accuracy tests verifying that warm start produces Q ≥ cold start Q on the three
  existing fixtures (Karate Club, Football, Polbooks) after small perturbations.
  Also test: warm start on an unperturbed graph (same graph) converges in fewer passes than cold.

### Claude's Discretion
- How to pass `InitialPartition` down from the detector to `reset()`:
  the cleanest approach (e.g., store it on the state struct temporarily, or pass as a param
  to `reset()`) is left to Claude's judgement at planning time.
- Whether to add a `WarmStart bool` convenience flag to `CommunityResult` indicating the run
  used warm start (useful for debugging): Claude decides based on plan complexity.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Existing Algorithm Files
- `graph/detector.go` — `CommunityDetector` interface, `CommunityResult`, `LouvainOptions`, `LeidenOptions`
- `graph/louvain.go` — Louvain `Detect()` loop, `phase1`, `reconstructPartition`, `buildSupergraph`
- `graph/louvain_state.go` — `louvainState`, `reset()`, `acquireLouvainState`, pool
- `graph/leiden.go` — Leiden `Detect()` loop, BFS refinement, supergraph aggregation
- `graph/leiden_state.go` — `leidenState`, `reset()`, `acquireLeidenState`, pool
- `graph/benchmark_test.go` — Existing benchmark pattern (10K-node fixture, `b.ResetTimer()`)
- `graph/accuracy_test.go` — NMI-based accuracy test pattern and helpers

### Prior Phase Context (for architectural consistency)
- `.planning/milestones/v1.0-phases/02-interface-louvain-core/02-CONTEXT.md`
- `.planning/milestones/v1.0-phases/03-leiden-implementation/03-CONTEXT.md`

### Key Prior Decisions (from STATE.md)
- `map[NodeID]int` as Partition — no external type
- `louvainState` wrapper pattern for phase1 reuse in Leiden
- `rand.New(src)` in `reset()` ensures identical RNG sequence; `rng.Seed()` causes divergence
- `bestSuperPartition` must be deep-copied; pointer sharing causes silent state corruption

No external specs — requirements fully captured in decisions above.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `louvainState.reset(g *Graph, seed int64)` — injection point; modify to accept optional initial partition
- `leidenState.reset(g *Graph, seed int64)` — same injection point
- `normalizePartition(p map[NodeID]int) map[NodeID]int` — check if it exists; if so, reuse for ID compaction after seeding
- `reconstructPartition(origNodes, nodeMapping, superPartition)` — unchanged; warm start only affects Phase 1 init
- `phase1(g, state, resolution, m)` — reads `state.partition` immediately; no change needed if `reset()` seeds it correctly

### Established Patterns
- Zero-value options with `if field == zero { field = default }` guards — extend `InitialPartition` as `nil`-guarded
- `sync.Pool` + `reset()` pattern — warm start must work with pooled state (pool returns a state that gets `reset()` called on it; the warm partition is passed into `reset()`, not stored on the pool object)
- `acquireLouvainState(g, seed)` wraps `Get()` + `reset()` — will need to accept optional partition or a separate warm-acquire variant

### Integration Points
- `louvainDetector.Detect()` calls `acquireLouvainState(currentGraph, seed)` then loops calling `state.reset()` — the warm partition needs to be injected only on the FIRST reset (iteration 0), not subsequent ones (those are supergraph passes)
- Same pattern in `leidenDetector.Detect()` with `acquireLeidenState()`

</code_context>

<specifics>
## Specific Ideas

- The user specifically frames this as "online community detection" — the usage pattern is:
  run Detect once cold, then re-run with the previous `CommunityResult.Partition` as seed
  whenever the graph is updated. The API should make this one-liner ergonomic:
  ```go
  result, _ = detector.Detect(g)         // cold start
  // ... graph updates ...
  opts.InitialPartition = result.Partition
  result, _ = detector.Detect(g)         // warm start
  ```
- The key insight: warm start only helps phase1 convergence on the FIRST supergraph level.
  Subsequent supergraph compression passes always start from scratch (supergraph topology changes).
  This is by design and is correct behavior.

</specifics>

<deferred>
## Deferred Ideas

- Streaming / event-driven graph update pipeline (add/remove edge → auto-redetect): own phase
- Directed graph warm start: blocked by directed graph support (v2 scope)
- Partial warm start: only re-seed communities near changed nodes — complex, future optimization

</deferred>

---

*Phase: 05-warm-start*
*Context gathered: 2026-03-30*
