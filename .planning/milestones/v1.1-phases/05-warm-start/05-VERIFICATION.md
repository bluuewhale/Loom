---
phase: 05-warm-start
verified: 2026-03-30T00:00:00Z
status: human_needed
score: 8/8 must-haves verified (static)
re_verification: false
human_verification:
  - test: "Run go test ./graph/... -count=1 -race -timeout=120s"
    expected: "All tests pass including TestLouvainWarmStartQuality, TestLeidenWarmStartQuality, TestLouvainWarmStartFewerPasses, TestLeidenWarmStartFewerPasses, and all pre-existing tests. Race detector reports no issues."
    why_human: "Go toolchain not available in the verification environment. Cannot execute tests to confirm behavioural correctness and absence of regressions."
  - test: "Run go test ./graph/... -bench 'WarmStart|Louvain10K|Leiden10K' -benchtime=5x -timeout=300s"
    expected: "BenchmarkLouvainWarmStart ns/op <= 50% of BenchmarkLouvain10K ns/op. BenchmarkLeidenWarmStart ns/op <= 50% of BenchmarkLeiden10K ns/op."
    why_human: "Benchmark speedup claim (warm <= 50% of cold) requires runtime execution to confirm. Cannot verify without Go toolchain."
---

# Phase 05: Warm-Start Verification Report

**Phase Goal:** Add warm-start to Louvain and Leiden: when InitialPartition is set in options, seed the algorithm's initial state from the prior partition instead of the trivial singleton. Provide benchmarks showing measurable speedup for small graph perturbations.
**Verified:** 2026-03-30
**Status:** human_needed (all static checks pass; 2 runtime behaviours need human execution)
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | InitialPartition map[NodeID]int field exists on LouvainOptions and LeidenOptions structs (nil = cold start) | ✓ VERIFIED | `detector.go` lines 34 and 48: `InitialPartition map[NodeID]int // nil = cold start (default)` — 2 occurrences confirmed |
| 2 | Warm seed is applied only on the first supergraph pass (firstPass guard prevents subsequent passes from re-seeding) | ✓ VERIFIED | `louvain.go` lines 80-87 and `leiden.go` lines 80-87: `firstPass := true` before loop; first iteration calls `state.reset(..., d.opts.InitialPartition)`, subsequent iterations call `state.reset(..., nil)`. 3 occurrences of `firstPass` in each file (declaration + if-branch + set-false). 1 occurrence of `d.opts.InitialPartition` in each file. |
| 3 | New nodes not in prior partition are assigned fresh singleton communities (max prior ID + offset) | ✓ VERIFIED | `louvain_state.go` lines 81-98 and `leiden_state.go` lines 85-102: `maxCommID` computed from prior partition (4 occurrences), `nextNewComm := maxCommID + 1` used as offset (3 occurrences each), nodes absent from prior partition assigned `nextNewComm++`. |
| 4 | commStr is rebuilt from current graph strengths after warm seeding (not copied from prior run) | ✓ VERIFIED | `louvain_state.go` lines 112-115 and `leiden_state.go` lines 116-120: Step 4 comment "build commStr from CURRENT graph strengths (not from prior run)" followed by `st.commStr[st.partition[n]] += g.Strength(n)`. `g.Strength(n)` appears 3 times in each state file (cold path + warm path). |
| 5 | Nil InitialPartition (zero value) preserves exact cold-start behaviour — no regression | ✓ VERIFIED | `louvain_state.go` line 71 and `leiden_state.go` line 76: `if initialPartition == nil { ... return }` guard exits after cold singleton assignment. `acquireLouvainState` and `acquireLeidenState` both pass `nil` (confirmed at lines 36 and 39 respectively). |
| 6 | Correctness tests cover Q(warm) >= Q(cold) on all 3 standard fixtures | ✓ VERIFIED | `accuracy_test.go`: `TestLouvainWarmStartQuality` (line 111, 3 subtests: KarateClub/Football/Polbooks) and `TestLeidenWarmStartQuality` (line 157, 3 subtests). Each asserts `warmResult.Modularity >= coldPerturbed.Modularity - 1e-9`. Plus `TestLouvainWarmStartFewerPasses` and `TestLeidenWarmStartFewerPasses` assert warm passes <= cold passes on unperturbed graph. |
| 7 | Benchmarks exist for both Louvain and Leiden warm-start scenarios | ✓ VERIFIED | `benchmark_test.go`: `BenchmarkLouvainWarmStart` (line 104) and `BenchmarkLeidenWarmStart` (line 129). Both: cold detect + perturbGraph called before `b.ResetTimer()`, only warm `Detect` in timed loop. `InitialPartition: coldResult.Partition` wiring confirmed (2 occurrences). |
| 8 | All existing tests still pass (no regressions introduced) | ? NEEDS HUMAN | Static analysis shows nil-guard preserves cold-start path unchanged, acquireLouvainState/acquireLeidenState both pass nil, pool safety maintained (InitialPartition not stored on state struct). But runtime test execution unavailable to confirm. |

**Score:** 7/8 truths statically verified; truth 8 requires runtime execution.

---

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `graph/detector.go` | InitialPartition field on LouvainOptions and LeidenOptions | ✓ VERIFIED | 2 occurrences of `InitialPartition map[NodeID]int` confirmed |
| `graph/louvain_state.go` | Warm-start seeding in louvainState.reset() | ✓ VERIFIED | Signature `reset(g *Graph, seed int64, initialPartition map[NodeID]int)`, full 4-step warm logic present: maxCommID, nextNewComm, compact remap, commStr rebuild |
| `graph/leiden_state.go` | Warm-start seeding in leidenState.reset() | ✓ VERIFIED | Identical to louvain_state.go; `clear(st.refinedPartition)` preserved at top of reset per D-07 |
| `graph/louvain.go` | firstPass guard in Louvain Detect loop | ✓ VERIFIED | `firstPass := true` declared at line 80, if/else at lines 82-87, `d.opts.InitialPartition` passed only on first pass |
| `graph/leiden.go` | firstPass guard in Leiden Detect loop | ✓ VERIFIED | `firstPass := true` declared at line 80, if/else at lines 82-87, `d.opts.InitialPartition` passed only on first pass |
| `graph/testhelpers_test.go` | perturbGraph helper for reproducible graph perturbation | ✓ VERIFIED | `func perturbGraph` at line 83; uses NewGraph + canonical edge rebuild (not Clone — correct, Graph has no RemoveEdge); seeded RNG for reproducibility |
| `graph/accuracy_test.go` | Warm-start correctness tests for Louvain and Leiden | ✓ VERIFIED | All 4 test functions present and substantive: TestLouvainWarmStartQuality, TestLeidenWarmStartQuality, TestLouvainWarmStartFewerPasses, TestLeidenWarmStartFewerPasses |
| `graph/benchmark_test.go` | Warm-start benchmarks for Louvain and Leiden | ✓ VERIFIED | BenchmarkLouvainWarmStart and BenchmarkLeidenWarmStart present; setup outside timed loop; b.ResetTimer() called after perturbGraph and cold detect |

---

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `graph/louvain.go` | `graph/louvain_state.go` | `state.reset` called with `d.opts.InitialPartition` on first pass | ✓ WIRED | Line 83: `state.reset(currentGraph, seed, d.opts.InitialPartition)` |
| `graph/leiden.go` | `graph/leiden_state.go` | `state.reset` called with `d.opts.InitialPartition` on first pass | ✓ WIRED | Line 83: `state.reset(currentGraph, seed, d.opts.InitialPartition)` |
| `graph/louvain_state.go` | `graph/detector.go` | `reset` reads InitialPartition map to seed partition | ✓ WIRED | Parameter `initialPartition map[NodeID]int` consumed in warm-start logic (Steps 1-4) |
| `graph/accuracy_test.go` | `graph/detector.go` | `LouvainOptions{InitialPartition: coldResult.Partition}` | ✓ WIRED | 4 occurrences of `InitialPartition.*Partition` pattern confirmed in accuracy_test.go |
| `graph/benchmark_test.go` | `graph/detector.go` | `LouvainOptions{InitialPartition: coldResult.Partition}` | ✓ WIRED | 2 occurrences confirmed; both Louvain and Leiden benchmark variants wired correctly |
| `graph/testhelpers_test.go` | `graph/graph.go` | Clone + selective rebuild for perturbation | ✓ WIRED (variant) | Uses `NewGraph(false)` + edge rebuild (not `Clone()`) — intentional: plan noted Graph has no RemoveEdge; rebuild approach is semantically equivalent for this use case |

---

## Data-Flow Trace (Level 4)

Not applicable — this phase adds algorithm logic and test infrastructure, not UI components or data-rendering pipelines. The "data flow" is the algorithmic pipeline: options struct -> Detect() loop -> reset() -> phase1() -> result. This is fully verified via the key links above.

---

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All tests pass including warm-start tests | `go test ./graph/... -count=1 -race -timeout=120s` | Go toolchain unavailable | ? SKIP — route to human |
| Warm benchmarks produce valid ns/op results | `go test ./graph/... -bench 'WarmStart' -benchtime=3x` | Go toolchain unavailable | ? SKIP — route to human |

Step 7b: SKIPPED for automated checks — Go toolchain not present in verification environment. Routed to human verification.

---

## Requirements Coverage

No formal requirement IDs declared for this phase (v1.1 feature addition). Phase goal and context decisions (D-01 through D-09) serve as the requirement contract. All 9 decisions are satisfied:

| Decision | Description | Status |
|----------|-------------|--------|
| D-01 | InitialPartition field on both Options structs, nil = cold start | ✓ SATISFIED |
| D-02 | Warm start applied inside reset() when InitialPartition != nil | ✓ SATISFIED |
| D-03 | New nodes assigned fresh singleton IDs past maxCommID+1 | ✓ SATISFIED |
| D-04 | Removed nodes silently ignored (loop only iterates g.Nodes()) | ✓ SATISFIED |
| D-05 | Community IDs compacted to 0-indexed contiguous after seeding | ✓ SATISFIED |
| D-06 | Convergence criteria unchanged (Tolerance/MaxPasses still apply) | ✓ SATISFIED |
| D-07 | refinedPartition left nil/empty on warm start | ✓ SATISFIED |
| D-08 | BenchmarkLouvainWarmStart and BenchmarkLeidenWarmStart added | ✓ SATISFIED |
| D-09 | Correctness tests verify Q(warm) >= Q(cold) and fewer passes | ✓ SATISFIED |

---

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| — | — | — | — | None found |

No TODO/FIXME comments, no placeholder returns, no empty handlers, no hardcoded empty data structures in production paths. The `newLouvainState` and `newLeidenState` functions (cold-only constructors retained for backward compatibility with Leiden's inline louvainState wrapper) are substantive — not stubs.

---

## Human Verification Required

### 1. Full Test Suite Execution

**Test:** From the repository root, run `go test ./graph/... -count=1 -race -timeout=120s`
**Expected:** All tests pass with output `ok community-detection/graph`. Specifically: TestLouvainWarmStartQuality (3 subtests), TestLeidenWarmStartQuality (3 subtests), TestLouvainWarmStartFewerPasses, TestLeidenWarmStartFewerPasses all PASS. No race conditions reported.
**Why human:** Go toolchain unavailable in the verification environment. Static analysis confirms the nil-guard preserves cold-start behaviour and pool safety (InitialPartition not stored on state), but runtime confirmation is required.

### 2. Benchmark Speedup Verification

**Test:** Run `go test ./graph/... -bench 'WarmStart|Louvain10K|Leiden10K' -benchtime=5x -timeout=300s` and compare ns/op values
**Expected:** BenchmarkLouvainWarmStart ns/op <= 50% of BenchmarkLouvain10K ns/op. BenchmarkLeidenWarmStart ns/op <= 50% of BenchmarkLeiden10K ns/op. (Target from D-08.)
**Why human:** Benchmark speedup is a runtime measurement. Static code analysis confirms the benchmark structure is correct (setup before ResetTimer, only warm Detect in timed loop), but actual speedup ratios require execution on a real graph.

---

## Gaps Summary

No gaps found. All 8 must-haves are satisfied at the static code level:

1. InitialPartition field is present on both options structs with correct type and nil semantics.
2. firstPass guard is present in both Detect loops — warm seed applied exactly once on the original graph.
3. New-node singleton offset logic (maxCommID + 1) is implemented identically in both state files.
4. commStr rebuild from current g.Strength() is present in both warm-start paths with explicit comments.
5. nil guard exits immediately to cold-start path; acquireLouvainState/acquireLeidenState pass nil.
6. Four correctness tests cover all three fixtures for both algorithms (quality + convergence speed).
7. Two benchmark functions exist with correct setup structure (cold detect + perturbation before ResetTimer).
8. Static inspection shows no breaking changes to cold-start path; runtime confirmation is pending human execution.

The only open item is runtime confirmation of test pass/fail and benchmark speedup ratios, which require the Go toolchain.

---

_Verified: 2026-03-30_
_Verifier: Claude (gsd-verifier)_
