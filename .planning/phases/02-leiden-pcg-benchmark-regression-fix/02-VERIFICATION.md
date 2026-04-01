---
phase: 02-leiden-pcg-benchmark-regression-fix
verified: 2026-04-01T08:00:00Z
status: passed
score: 3/3 must-haves verified
---

# Phase 02: Leiden PCG Benchmark Regression Fix — Verification Report

**Phase Goal:** Eliminate per-community map allocations in refinePartition — the dominant
Leiden-specific allocation source — to bring Leiden 10K allocs/op to Louvain parity.
**Verified:** 2026-04-01T08:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Leiden 10K allocs/op drops from 58,220 to ≤ 46,500 (seed 110 PCG 4-pass) | VERIFIED | Measured: 45,872 / 45,919 / 45,823 allocs/op (avg ~45,871). Target 46,500. Baseline 58,220. −21.2% |
| 2 | All existing tests pass | VERIFIED | `go test ./graph/... -count=1 -timeout 120s` exits 0 in 12.297s |
| 3 | No public API signature changes | VERIFIED | All public types and functions (NewLeiden, LeidenOptions, CommunityDetector.Detect, NewLouvain, LouvainOptions, NewEgoSplitting, EgoSplittingOptions, Graph methods) have unchanged signatures. Changes are confined to unexported internals: `refinePartitionInPlace`, `commNodePair` type, and new `leidenState` fields |

**Score: 3/3 truths verified**

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `graph/leiden.go` | `commNodePair` type + `refinePartitionInPlace` replacing `refinePartition` | VERIFIED | `type commNodePair struct` at line 11; `func refinePartitionInPlace` at line 238; old `func refinePartition` absent |
| `graph/leiden_state.go` | `commBuildPairs []commNodePair`, `inCommBits []bool`, `visitedBits []bool` fields | VERIFIED | All three fields present at lines 22–24; `commSortedPairs` also present at line 29; initialized in pool `New()` at line 48 |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `leiden.go:runOnce` | `refinePartitionInPlace` | direct call at line 163 | WIRED | `refinePartitionInPlace(currentGraph, &csr, state.partition, state)` — passes `leidenState` for scratch reuse |
| `refinePartitionInPlace` | `st.inCommBits` / `st.visitedBits` | CSR-indexed `[]bool` | WIRED | Lazy-grown at lines 243–246; used in BFS at lines 323, 331, 336, 348 |
| `refinePartitionInPlace` | `st.commBuildPairs` / `st.commSortedPairs` | counting-sort scatter | WIRED | Pairs built at line 263; prefix-sum and scatter at lines 282–294; output consumed by BFS loop at line 329 |
| `refinePartitionInPlace` | `st.refinedPartition` | `clear()` + repopulate | WIRED | `clear(st.refinedPartition)` at line 304; `st.refinedPartition[cur] = nextID` at line 342 |
| `leiden_state.go:pool New()` | `commBuildPairs` | `make([]commNodePair, 0, 128)` at line 48 | WIRED | Initialised in pool constructor; `inCommBits`, `visitedBits`, `commCountScratch` grown lazily in `refinePartitionInPlace` |

---

### Data-Flow Trace (Level 4)

Not applicable. This phase modifies internal algorithmic structures (allocation hot paths,
per-community scratch memory), not user-visible rendering. The observable outcome is the
benchmark allocation counter, verified directly via `-benchmem`.

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All existing tests pass | `go test ./graph/... -count=1 -timeout 120s` | `ok github.com/bluuewhale/loom/graph 12.297s` | PASS |
| BenchmarkLeiden10K allocs/op — run 1 | `go test -bench=BenchmarkLeiden10K$ -benchmem -count=3 -run='^$' ./graph/...` | 45,872 allocs/op | PASS — target ≤ 46,500, baseline 58,220 |
| BenchmarkLeiden10K allocs/op — run 2 | same | 45,919 allocs/op | PASS |
| BenchmarkLeiden10K allocs/op — run 3 | same | 45,823 allocs/op | PASS |

**Average:** 45,871 allocs/op — −21.3% vs 58,220 baseline. Target ≤ 46,500 met by a margin of 629 allocs/op.

Note on ns/op: runs measured ~60ms / ~60ms / ~65ms. The higher third run is normal M4 thermal
variance at 16 iterations; the alloc count (the primary target) is deterministic and stable
across all three runs.

---

### Requirements Coverage

| Requirement | Status | Evidence |
|-------------|--------|----------|
| Leiden 10K allocs/op ≤ 46,500 (seed 110, PCG 4-pass) | MET | Measured avg ~45,871 across 3 runs. −21.3% vs 58,220 baseline. |
| All existing tests pass | MET | `go test ./graph/... -count=1` exits 0 (12.297s). |
| No public API signature changes | MET | `refinePartitionInPlace` is unexported. `commNodePair` is unexported. All new `leidenState` fields are unexported. No changes to detector.go public surface. |

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | — | — | — | — |

No TODOs, FIXMEs, stubs, placeholder returns, or hardcoded empty data found in `leiden.go` or `leiden_state.go`.

---

### Note on EgoSplitting Test Flakiness

`TestEgoSplittingOmegaIndex/KarateClub` is a known flaky test unrelated to this phase's
changes. The flakiness originates from non-deterministic map iteration order in
`ego_splitting.go` — a pre-existing condition not introduced or worsened by Phase 2.
The test passed in the run above (`ok` in 12.297s), confirming no regression. If it fails
in a future run, re-running `go test ./graph/... -count=1` is sufficient — the flakiness
is stochastic and not caused by `refinePartitionInPlace`.

---

### Human Verification Required

None. All verifiable behaviors were checked programmatically. Alloc counts are directly
observable from benchmark output. API surface is statically verifiable via grep.

---

### Gaps Summary

No gaps. All three success criteria are met:

1. **Alloc reduction:** 58,220 → avg 45,871 allocs/op (−21.3%), comfortably under the ≤ 46,500 target.
2. **Tests:** full suite passes in 12.297s.
3. **API stability:** all changes are unexported — zero public surface impact.

The SUMMARY's reported outcome (45,938 allocs/op, −21%) is consistent with the independently
measured benchmark results (45,872 / 45,919 / 45,823).

---

_Verified: 2026-04-01T08:00:00Z_
_Verifier: Claude (gsd-verifier)_
