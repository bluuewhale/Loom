---
phase: 04-performance-hardening-benchmark-fixtures
verified: 2026-03-29T00:00:00Z
status: gaps_found
score: 8/9 must-haves verified
gaps:
  - truth: "Repeated same-size graph calls show 0 allocs/op after warmup"
    status: failed
    reason: "bench-baseline.txt and live benchmark runs both show ~48773 allocs/op for Louvain10K and ~66552 allocs/op for Leiden10K across all iterations — pool warmup does not reduce allocs to 0. The sync.Pool reuses the top-level state struct but the algorithm still allocates extensively per iteration (map and slice growth in phase1, buildSupergraph, etc.)."
    artifacts:
      - path: "graph/louvain_state.go"
        issue: "louvainStatePool and neighborBuf dirty-list are present, but phase1 still makes per-node map allocations beyond the candidateBuf optimization; broader algorithm allocations (candidateBuf growth, supergraph construction) are not pooled."
      - path: "graph/benchmark_test.go"
        issue: "BenchmarkLouvain10K_Allocs confirms 48773 allocs/op stable across 5 runs — not 0."
    missing:
      - "Eliminate or pool remaining per-iteration allocations in phase1 (candidateBuf growth, neighbor weight map rewrites) and buildSupergraph so that allocs/op converges to 0 on repeated same-size calls."
human_verification:
  - test: "NMI thresholds on Football and Polbooks"
    expected: "NMI >= 0.5 for both Louvain and Leiden on Football and Polbooks benchmarks"
    why_human: "Tests pass with fixed seeds but NMI accuracy depends on non-deterministic algorithm internals; a human should verify the t.Log output shows plausible NMI values (0.5-0.9 range) rather than boundary-passing values."
---

# Phase 04: Performance Hardening + Benchmark Fixtures Verification Report

**Phase Goal:** Both algorithms meet the <100ms / 10K-node target, concurrent use is race-free, and accuracy is validated on three benchmark graphs
**Verified:** 2026-03-29
**Status:** gaps_found
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #  | Truth                                                                      | Status      | Evidence                                                                      |
|----|----------------------------------------------------------------------------|-------------|-------------------------------------------------------------------------------|
| 1  | Football fixture has 115 nodes and 613 edges                               | VERIFIED    | python3 count: 613 edges, 115 unique nodes, max node ID 114                   |
| 2  | Polbooks fixture has 105 nodes and 441 edges                               | VERIFIED    | python3 count: 441 edges, 105 unique nodes, max node ID 104, 3 communities    |
| 3  | nmi() helper is accessible from all test files in package graph            | VERIFIED    | Lives in testhelpers_test.go; absent from leiden_test.go and louvain_test.go  |
| 4  | Louvain and Leiden both achieve NMI validation on Football and Polbooks    | VERIFIED    | 4 NMI tests in accuracy_test.go; `go test -race ./graph/... -count=1` passes  |
| 5  | Louvain and Leiden both achieve Q > 0.35 on Karate Club                   | VERIFIED    | TestLouvainKarateClubNMI in accuracy_test.go; existing Leiden test passes     |
| 6  | All 8 edge cases pass for both Louvain and Leiden                          | VERIFIED    | 4 new Louvain edge-cases added; all pass under -race                          |
| 7  | BenchmarkLouvain10K reports < 100ms/op                                     | VERIFIED    | Live run: ~48ms/op (benchtime=3x); baseline: ~50ms/op                        |
| 8  | BenchmarkLeiden10K reports < 100ms/op                                      | VERIFIED    | Live run: ~57ms/op (benchtime=3x); baseline: ~56ms/op                        |
| 9  | Repeated same-size graph calls show 0 allocs/op after warmup               | FAILED      | ~48773 allocs/op (Louvain) and ~66552 allocs/op (Leiden) on every iteration  |
| 10 | go test -race ./graph/... passes with zero data race reports               | VERIFIED    | `go test ./graph/... -count=1 -race` exits ok in 1.254s                       |
| 11 | Concurrent Detect calls on different graphs produce no races               | VERIFIED    | TestConcurrentDetect in benchmark_test.go passes under -race                  |
| 12 | benchstat baseline file exists for regression tracking                     | VERIFIED    | bench-baseline.txt exists at project root with 15 benchmark lines             |

**Score:** 11/12 truths verified (8/9 plan must-haves — the "0 allocs" truth is the gap)

### Required Artifacts

| Artifact                          | Expected                                              | Status    | Details                                                          |
|-----------------------------------|-------------------------------------------------------|-----------|------------------------------------------------------------------|
| `graph/testdata/football.go`      | 613 edges, 115-node partition, 12 communities         | VERIFIED  | FootballEdges (613), FootballPartition (115 entries, IDs 0-11)   |
| `graph/testdata/polbooks.go`      | 441 edges, 105-node partition, 3 communities          | VERIFIED  | PolbooksEdges (441), PolbooksPartition (105 entries, IDs 0-2)    |
| `graph/testhelpers_test.go`       | nmi(), uniqueCommunities(), buildGraph(), groundTruth | VERIFIED  | All 4 helpers present at lines 11, 52, 61, 70                   |
| `graph/accuracy_test.go`          | 5 NMI accuracy tests                                  | VERIFIED  | TestLouvainKarateClubNMI + 4 Football/Polbooks tests             |
| `graph/louvain_test.go`           | 8 edge cases including Giant, ZeroRes, Complete, Loop | VERIFIED  | All 4 new cases at lines 247, 283, 318, 349                     |
| `graph/louvain_state.go`          | louvainStatePool, acquire/release, neighborBuf        | VERIFIED  | Pool, acquire, release, neighborBuf, neighborDirty all present   |
| `graph/leiden_state.go`           | leidenStatePool, acquire/release, neighborBuf         | VERIFIED  | Pool, acquire, release, neighborBuf, neighborDirty all present   |
| `graph/benchmark_test.go`         | BenchmarkLouvain10K, BenchmarkLeiden10K, TestConcurrentDetect | VERIFIED | All 3 present                                          |
| `bench-baseline.txt`              | benchstat baseline in project root                    | VERIFIED  | Exists with 15 measurement rows                                  |

### Key Link Verification

| From                      | To                        | Via                              | Status    | Details                                              |
|---------------------------|---------------------------|----------------------------------|-----------|------------------------------------------------------|
| graph/accuracy_test.go    | graph/testdata/football.go| testdata.FootballEdges           | VERIFIED  | Pattern `testdata\.FootballEdges` found in file      |
| graph/accuracy_test.go    | graph/testdata/polbooks.go| testdata.PolbooksEdges           | VERIFIED  | Pattern `testdata\.PolbooksEdges` found in file      |
| graph/accuracy_test.go    | graph/testhelpers_test.go | nmi() call                       | VERIFIED  | Pattern `nmi(res.Partition` found in file            |
| graph/louvain.go          | graph/louvain_state.go    | acquireLouvainState              | VERIFIED  | Line 74: `state := acquireLouvainState(currentGraph, seed)` |
| graph/leiden.go           | graph/leiden_state.go     | acquireLeidenState               | VERIFIED  | Line 74: `state := acquireLeidenState(currentGraph, seed)`  |
| graph/louvain.go          | graph/louvain_state.go    | neighborDirty dirty-list in phase1 | VERIFIED | neighborDirty used at lines 184-205 of louvain.go   |

### Data-Flow Trace (Level 4)

Not applicable — this phase produces algorithm infrastructure and test code, not UI components rendering dynamic data.

### Behavioral Spot-Checks

| Behavior                          | Command                                                      | Result                    | Status  |
|-----------------------------------|--------------------------------------------------------------|---------------------------|---------|
| All tests pass with race detector | `go test ./graph/... -count=1 -race`                         | ok in 1.254s              | PASS    |
| Louvain 10K under 100ms           | `go test -bench=BenchmarkLouvain10K -benchmem -benchtime=3x` | ~48ms/op                  | PASS    |
| Leiden 10K under 100ms            | `go test -bench=BenchmarkLeiden10K -benchmem -benchtime=3x`  | ~57ms/op                  | PASS    |
| 0 allocs/op after warmup (Louvain)| `bench-baseline.txt` allocs column                           | 48773 allocs/op all runs  | FAIL    |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                 | Status       | Evidence                                                         |
|-------------|-------------|-----------------------------------------------------------------------------|--------------|------------------------------------------------------------------|
| PERF-01     | 04-02       | <100ms on 10K-node / ~50K-edge graph, single goroutine, -bench measurement  | SATISFIED    | Louvain ~48ms, Leiden ~57ms measured live                        |
| PERF-02     | 04-02       | Concurrent-safe — no race on distinct *Graph instances                       | SATISFIED    | TestConcurrentDetect passes under -race; full suite -race green  |
| PERF-03     | 04-02       | sync.Pool reuse for louvainState — minimize allocations on repeated calls    | PARTIAL      | Pool integrated and functional; but allocs not reduced to 0/op   |
| PERF-04     | 04-02       | go test -race passes                                                         | SATISFIED    | Confirmed: `go test ./graph/... -count=1 -race` exits ok         |
| TEST-01     | 04-01       | Karate Club — Louvain, Leiden both Q > 0.35                                  | SATISFIED    | TestLouvainKarateClubNMI + pre-existing Leiden accuracy test     |
| TEST-02     | 04-01       | Football network (115 nodes, 613 edges) fixture + NMI validation             | SATISFIED    | football.go verified; TestLouvainFootballNMI + TestLeidenFootballNMI pass |
| TEST-03     | 04-01       | Polbooks fixture (105 nodes, 441 edges)                                      | SATISFIED    | polbooks.go verified; TestLouvainPolbooksNMI + TestLeidenPolbooksNMI pass |
| TEST-04     | 04-01       | 8 edge cases: empty, single-node, disconnected, giant+singletons, 2-node, zero-resolution, complete, self-loop | SATISFIED | All 8 cases present and passing in louvain_test.go |
| TEST-05     | 04-02       | benchstat baseline for performance regression prevention                     | SATISFIED    | bench-baseline.txt in project root with 15 measurement rows      |

All 9 requirement IDs from PLAN frontmatter accounted for. No orphaned requirements detected.

### Anti-Patterns Found

| File                    | Line  | Pattern                  | Severity | Impact                                               |
|-------------------------|-------|--------------------------|----------|------------------------------------------------------|
| graph/benchmark_test.go | 89-99 | BenchmarkLouvain10K_Allocs does not assert 0 allocs | Info | The benchmark exists but cannot enforce the 0-alloc target programmatically — it just reports; no automated failure when allocs > 0 |

No TODO/FIXME/placeholder patterns found in phase-modified files. No empty return stubs detected.

### Human Verification Required

#### 1. NMI Threshold Plausibility

**Test:** Run `go test ./graph/... -v -run "TestLouvainFootball|TestLeidenFootball|TestLouvainPolbooks|TestLeidenPolbooks|TestLouvainKarateClubNMI" -count=1` and inspect t.Log output for actual NMI and Q values.
**Expected:** Karate NMI >= 0.7, Football NMI >= 0.5, Polbooks NMI >= 0.5 — with plausible values (0.5-0.9 range), not boundary-skimming values that would indicate seed-sensitivity.
**Why human:** Tests pass/fail is automated, but a human should confirm the NMI values are in a healthy range, not barely scraping the threshold due to lucky seed selection.

### Gaps Summary

One gap blocks the "0 allocs/op after warmup" must-have truth from 04-02-PLAN.md:

The sync.Pool correctly pools the top-level `louvainState` and `leidenState` structs, and the neighborBuf dirty-list avoids `clear()` on the whole map each iteration. However, the algorithm still makes approximately 48,773 allocations per 10K-node Louvain run and 66,552 per Leiden run — consistent across all benchmark iterations (no reduction after warmup). The primary sources are: (1) map growth/rehashing inside the algorithm body (communityToNodes, candidateSet in phase1, supergraph construction in buildSupergraph), and (2) slice growth in candidateBuf and dirty-list expansion.

The performance target (PERF-01: <100ms) IS met. Only the secondary goal of 0-alloc reuse is not. PERF-03 is therefore partially satisfied: the Pool infrastructure is in place, but the alloc-minimization outcome is not achieved.

This gap does not block the primary phase goal (both algorithms meet <100ms, race-free, accuracy validated). It is a secondary optimization target that was listed as a must-have truth in the plan but is not required by any REQUIREMENTS.md entry beyond PERF-03 (which says "minimize allocations", not "zero allocations").

---

_Verified: 2026-03-29_
_Verifier: Claude (gsd-verifier)_
