---
phase: 5
reviewers: [claude-self]
reviewed_at: 2026-03-30T00:00:00Z
plans_reviewed: [05-01-PLAN.md, 05-02-PLAN.md]
note: "gemini and codex blocked by Superset PATH restrictions in this worktree; claude self-review only"
---

# Cross-AI Plan Review — Phase 05: Warm Start

> **Note:** External AI CLIs (gemini, codex) are blocked by Superset PATH restrictions
> in this worktree environment. This review was produced by Claude (self-review).
> Independence is limited — treat as a structured checklist, not an adversarial audit.

---

## Claude Self-Review

> **Bias caveat:** This review is produced by the same model that authored the plans.
> It cannot substitute for independent AI review. Treat findings as a checklist, not
> an adversarial audit.

### Summary

Both plans are well-scoped and technically sound. Plan 05-01 correctly addresses the
pool-safety pitfall (do not store `initialPartition` on the state struct) and the
supergraph-pass pitfall (firstPass guard). Plan 05-02 correctly places benchmark setup
outside `b.ResetTimer()`. The main risks are: (1) the 50% speedup target may not be met
for Leiden because its BFS refinement pass dominates wall time independently of the
initial partition; (2) `perturbGraph` silently produces a multigraph if `AddEdge` does
not deduplicate — tests could pass with inflated edge counts.

### Strengths

- **Pool safety preserved**: `initialPartition` passed as a parameter to `reset()`, never
  stored on the state struct. This correctly avoids the classic pool-object mutation bug.
- **firstPass guard**: Warm seed applied only on the first supergraph level (original
  NodeIDs). Subsequent supergraph passes always cold-reset. This is the correct semantics.
- **commStr rebuilt from current graph**: `g.Strength(n)` is recomputed after seeding,
  not copied from prior run. Avoids stale strength values on perturbed graphs.
- **Zero breaking change**: nil `InitialPartition` preserves cold-start behavior
  identically; existing callers need no changes.
- **D-04 simplicity**: Silently ignoring removed nodes (stale keys in InitialPartition)
  is the right call — no error surface, no overhead.
- **Benchmark design**: Cold detect and perturbation outside `b.ResetTimer()`. Only the
  warm `Detect` call is measured. Correct controlled benchmarking.
- **Test coverage**: 4 correctness tests covering both algorithms, both quality and
  pass-count dimensions.

### Concerns

**Plan 05-01:**

- **[HIGH] commStr compaction off-by-one risk**: After the compact remapping loop, the
  warm-seed code does `st.commStr[st.partition[n]] += g.Strength(n)`. If `commStr` is a
  slice (not a map), it must be pre-sized to `len(nodes)` before the strength rebuild.
  The plan doesn't verify that `commStr` is cleared/zeroed between the compact step and
  the strength rebuild — stale values from the previous pool use could accumulate.
  Verify that `clear(st.commStr)` (or equivalent) precedes the strength rebuild.

- **[HIGH] commStr slice capacity vs community count**: The compacted IDs run 0..K-1
  where K = number of unique communities in `initialPartition` (≤ `len(nodes)`). But
  the slice `commStr` is likely sized to `len(nodes)` (one entry per node in cold start).
  If K < len(nodes), this is fine. But the plan must confirm that `commStr` is large
  enough before indexing into it by community ID. If the slice is pre-allocated to
  `len(nodes)` (which it should be from the pool), this is safe.

- **[MEDIUM] Determinism of remap across runs**: The remap loop iterates `nodes` (sorted)
  to assign contiguous IDs. This is deterministic given fixed graph topology. However,
  two warm runs with the same `InitialPartition` on the same graph will always produce
  the same compacted IDs — this is good. Confirm that the `nodes` slice is sorted before
  the remap loop (it is, since `reset()` sorts nodes early).

- **[MEDIUM] Leiden `refinedPartition` and warm start interaction**: D-07 says
  `refinedPartition` is left nil. But `leidenState` has `refinedPartition` as a map.
  If the pool returns a state with a non-nil `refinedPartition` from a prior cold run,
  and `reset()` only calls `clear(st.refinedPartition)` (not nil assignment), the map
  still has memory allocated. This is fine for correctness but worth confirming that
  `clear()` is called before the warm-seed path too.

- **[LOW] No validation of InitialPartition values**: If a caller passes community IDs
  that are very large (e.g., 10^9), `nextNewComm = maxCommID + 1` would also be large,
  and the compact step would still produce 0..K-1. This is correct behavior, just noting
  that the compact step is load-bearing for correctness with arbitrary prior IDs.

**Plan 05-02:**

- **[HIGH] perturbGraph multigraph risk**: `AddEdge(a, b, 1.0)` in the "add random edges"
  loop does not check for existing edges. If the graph type accumulates parallel edges
  (adds weight to existing edge vs creates duplicate), the perturbed graph's topology
  differs from what the test intends. Verify that `AddEdge` on an existing edge either
  deduplicates or adds weight (neither is wrong, but the test logic should account for
  this). If it creates parallel edges, `Neighbors()` will return duplicates which inflates
  edge count and distorts modularity.

- **[HIGH] perturbGraph self-loop guard is incomplete**: The add loop checks `a != b` but
  does not prevent adding edges that already exist in `allEdges` (edges that weren't
  removed). This means nAdd edges are always added on top of surviving edges, potentially
  creating multigraph topology. The comment says "skip self-loops and duplicates" but the
  code only skips self-loops.

- **[MEDIUM] Benchmark warm speedup target (50%) may not hold for Leiden**: Leiden's BFS
  refinement pass is an O(E) sweep that runs regardless of initial partition quality.
  The warm-start benefit accrues primarily in the local-move phase, but for a 1%
  perturbation on a 10K-node graph, the BFS refinement may dominate wall time. The 50%
  target is ambitious. Consider documenting this as an aspirational target and assert
  only that warm ns/op < cold ns/op (any improvement), not the specific 50% threshold.

- **[MEDIUM] `TestLouvainWarmStartFewerPasses` brittleness**: On a very small graph
  (Karate Club, 34 nodes), warm start on an identical graph may still converge in the
  same number of passes as cold start (both reach the optimum in one pass). The assertion
  `warm passes ≤ cold passes` would pass trivially (0 ≤ 0 or 1 ≤ 1) without actually
  testing anything meaningful. Consider logging an explicit note if both pass counts are
  equal, and/or use a larger fixture (Football, 115 nodes) for this test.

- **[LOW] bench10K perturbation reused across b.N iterations**: The same `perturbed` graph
  is used for all `b.N` iterations. This is intentional (controlled benchmark) but means
  the benchmark measures best-case warm start: the cached `InitialPartition` from the
  original graph is maximally close to the perturbed graph. Real-world warm start would
  accumulate drift over multiple sequential updates.

### Suggestions

1. **Confirm commStr is zeroed before strength rebuild**: Add an explicit `clear(st.commStr)`
   call (or verify it's already in the warm-seed path) before the `g.Strength` accumulation
   loop to prevent stale values from prior pool use.

2. **Fix perturbGraph duplicate-edge check**: Add a set of existing edges before the
   "add random edges" loop, and skip additions that already exist in the rebuilt graph.
   This ensures the perturbed graph has exactly `original - nRemove + nAdd` distinct edges.

3. **Relax warm-start benchmark assertion**: Change the 50% ns/op target to an informational
   log rather than a hard assertion. Document it as a design goal in comments:
   `// Target: warm ≤ 50% of cold ns/op for small perturbations (Louvain); may be less for Leiden.`

4. **Add edge case test: empty InitialPartition**: A `map[NodeID]int{}` (non-nil but empty)
   is different from `nil`. Currently D-01 says "nil = cold start" — but an empty map
   would take the warm-start code path and assign all nodes to fresh singletons (effectively
   cold start with extra steps). Consider either: (a) treat empty map as nil in the reset()
   guard, or (b) add a test confirming empty map behavior matches expectations.

5. **Document the firstPass pattern in a comment**: The warm-start-only-on-first-pass
   behavior is subtle. A one-line comment in the Detect loop (`// warm start applies only
   to the original graph, not to supergraph passes`) would help future maintainers
   understand the invariant.

### Risk Assessment: **LOW-MEDIUM**

The implementation design is fundamentally correct. The pool-safety and firstPass invariants
are the critical correctness properties and both are explicitly addressed. The main risks are
implementation-level (commStr zeroing, perturbGraph multigraph) rather than design-level.
These are catchable during test execution. The 50% benchmark target is the only potentially
misleading specification — if Leiden warm start achieves only 30% speedup, the benchmark
would fail despite correct implementation.

---

## Consensus Summary

*(Single reviewer — no multi-reviewer consensus available)*

### Key Strengths

- Pool safety and firstPass guard are the two highest-stakes design decisions; both are
  handled correctly.
- Zero breaking change via nil guard is the right API choice.
- Benchmark setup correctly isolated outside the timed loop.

### Priority Concerns

1. **[HIGH] commStr zeroing before warm-seed strength rebuild** — verify `clear(st.commStr)`
   precedes the strength accumulation in warm-seed path.
2. **[HIGH] perturbGraph multigraph risk** — `AddEdge` in add loop may create parallel
   edges; add duplicate-edge guard.
3. **[MEDIUM] 50% Leiden speedup target** — may be too aggressive; treat as aspirational.

### Divergent Views

N/A — single reviewer.
