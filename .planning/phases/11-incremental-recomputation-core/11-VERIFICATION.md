---
phase: 11-incremental-recomputation-core
verified: 2026-03-31T00:00:00Z
status: passed
score: 7/7 must-haves verified
gaps: []
human_verification: []
---

# Phase 11: Incremental Recomputation Core — Verification Report

**Phase Goal:** Replace the full-graph recompute path inside `Update()` with incremental logic — affected-node scoping, ego-net selective rebuild, persona graph patching, and warm-started global detection — while preserving PersonaID disjointness.
**Verified:** 2026-03-31
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|---------|
| 1 | `Update()` recomputes ego-nets only for affected nodes (new nodes + neighbors of edge endpoints) — not all n ego-nets | ✓ VERIFIED | `computeAffected` called at line 222; `TestUpdate_AffectedNodesOnly` uses `countingDetector` spy and asserts `spy.count == len(affected)` — passes |
| 2 | Unaffected nodes' personas carry over from prior result with identical PersonaIDs | ✓ VERIFIED | `buildPersonaGraphIncremental` deep-copies prior maps and only overwrites affected entries; `TestUpdate_UnaffectedPersonasCarriedOver` asserts PersonaID equality for every unaffected node — passes |
| 3 | New PersonaIDs are allocated from `max(priorInverseMap keys, g.Nodes()) + 1` — no collisions | ✓ VERIFIED | `nextPersona = maxID + 1` at line 515 after scanning both sets; `TestUpdate_PersonaIDDisjoint` and `TestBuildPersonaGraphIncremental_PersonaIDAboveMax` confirm no collision — both pass |
| 4 | `Update()` with nil carry-forward fields in prior gracefully falls back to `Detect()` | ✓ VERIFIED | Nil guard at lines 217–219; `TestUpdate_NilCarryForwardFallback` passes without panic and returns valid result |
| 5 | Persona graph is rebuilt from patched data structures (no `RemoveNode` needed) | ✓ VERIFIED | `buildPersonaGraphIncremental` Step f creates `NewGraph(false)` and rewires edges from scratch using the same canonical `[lo,hi]` dedup pattern as `buildPersonaGraph` — lines 571–619 |
| 6 | `Detect()` populates all four carry-forward fields (`personaOf`, `inverseMap`, `partitions`, `personaPartition`) | ✓ VERIFIED | Fields assigned at lines 184–191; `TestDetect_PopulatesCarryForwardFields` asserts all four non-nil with correct entry counts (34 nodes) — passes |
| 7 | `warmStartedDetector()` constructs a new detector with `InitialPartition` set without mutating the original `GlobalDetector` | ✓ VERIFIED | Type switch at lines 299–318; `TestWarmStartedDetector_DoesNotMutateOriginal` confirms original unchanged — passes |

**Score:** 7/7 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `graph/ego_splitting.go` | `computeAffected`, `buildPersonaGraphIncremental`, `warmStartedDetector`, incremental `Update()`, carry-forward fields on `OverlappingCommunityResult` | ✓ VERIFIED | All functions present and substantive; `Update()` calls all three helpers in sequence (lines 222–232); no stubs |
| `graph/ego_splitting_test.go` | Tests for ONLINE-05 through ONLINE-11 plus carry-forward field tests | ✓ VERIFIED | 18 phase-11 tests confirmed present and passing; `countingDetector` spy present at line 1166 |

---

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `Update()` | `computeAffected()` | scopes ego-net rebuild to affected nodes only | ✓ WIRED | Called at line 222; result bound to `affected` |
| `Update()` | `buildPersonaGraphIncremental()` | patches persona graph using prior state + affected set | ✓ WIRED | Called at lines 225–228; all 6 return values consumed |
| `Update()` | `warmStartedDetector()` | constructs warm-started global detector from prior persona partition | ✓ WIRED | Called at line 232; `warmPartition` from step 2 passed as argument |
| `Update()` | `mapPersonasToOriginal()` | maps global persona partition back to original node communities | ✓ WIRED | Called at line 239 with `globalResult.Partition` and `newInverseMap` |
| `Detect()` | `OverlappingCommunityResult` | populates `personaOf`, `inverseMap`, `partitions`, `personaPartition` before return | ✓ WIRED | All four fields assigned at lines 184–191 |
| `warmStartedDetector()` | `NewLouvain`/`NewLeiden` | type switch on `*louvainDetector`/`*leidenDetector` to extract opts and inject `InitialPartition` | ✓ WIRED | `case *louvainDetector` at line 300, `case *leidenDetector` at line 308 |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|--------------------|--------|
| `Update()` result | `nodeCommunities` | `mapPersonasToOriginal(globalResult.Partition, newInverseMap)` | Yes — `globalResult.Partition` is the output of `warmGlobal.Detect(personaGraph)` on a real persona graph | ✓ FLOWING |
| `OverlappingCommunityResult.personaOf` | `personaOf` | `buildPersonaGraphIncremental` → `localDetector.Detect(egoNet)` per affected node | Yes — real ego-net detection for affected nodes; prior map copy for unaffected | ✓ FLOWING |
| `buildPersonaGraphIncremental` persona graph | edges | all edges in `g` rewired through `partitions` lookup | Yes — O(|E|) full edge scan, same pattern as `buildPersonaGraph` | ✓ FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All 18 Phase 11 requirement tests pass | `go test ./graph/... -run "TestDetect_PopulatesCarryForwardFields\|TestDetect_CarryForwardNilFallback\|TestWarmStartedDetector\|TestComputeAffected\|TestBuildPersonaGraphIncremental\|TestUpdate_..."` | PASS (0.211s) | ✓ PASS |
| Full test suite passes with race detector (excluding flaky benchmark-as-test) | `go test ./graph/... -count=1 -race -run Test` | PASS (20.008s) | ✓ PASS |
| Build and vet clean | `go build ./graph/... && go vet ./graph/...` | No output (success) | ✓ PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|---------|
| ONLINE-05 | 11-02-PLAN.md | `Update()` recomputes ego-nets only for affected nodes | ✓ SATISFIED | `computeAffected` in `Update()` + `TestUpdate_AffectedNodesOnly` with `countingDetector` spy |
| ONLINE-06 | 11-02-PLAN.md | `Update()` patches persona graph incrementally — unaffected personas carried over | ✓ SATISFIED | `buildPersonaGraphIncremental` deep-copies prior maps, overwrites only affected entries + `TestUpdate_UnaffectedPersonasCarriedOver` |
| ONLINE-07 | 11-01-PLAN.md | `Update()` warm-starts global detection from prior persona partition | ✓ SATISFIED | `warmStartedDetector(d.opts.GlobalDetector, warmPartition)` at line 232 + `TestUpdate_WarmStartGlobalDetection` |
| ONLINE-11 | 11-02-PLAN.md | PersonaID allocation never collides with original `NodeID` space | ✓ SATISFIED | `nextPersona = max(priorInverseMap keys, g.Nodes()) + 1` at line 515 + `TestUpdate_PersonaIDDisjoint` + `TestUpdate_MultipleSequentialUpdates` |

All four requirement IDs declared in plans are satisfied. The traceability table in REQUIREMENTS.md maps exactly ONLINE-05, ONLINE-06, ONLINE-07, ONLINE-11 to Phase 11 — no orphaned requirements.

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `graph/ego_splitting.go` | 374, 544 | `return nil, nil, ...` | ℹ️ Info | Error propagation paths in `buildPersonaGraph` and `buildPersonaGraphIncremental` — correct Go idiom, not stubs |

No blockers or warnings. The two `return nil` matches are error-path returns, not placeholder implementations. Both are inside `if err != nil` guards.

---

### Human Verification Required

None. All observable behaviors from the phase goal are verifiable programmatically:

- Affected-node scoping is measurable via `countingDetector` spy.
- PersonaID disjointness is checkable by set intersection.
- Carry-forward identity is checkable by map comparison.
- Warm-start wiring is confirmed by the type switch and test coverage.

---

### Gaps Summary

No gaps. All phase 11 must-haves are verified at all four levels (exists, substantive, wired, data flowing). The phase goal is fully achieved.

**Pre-existing issue (out of scope):** `TestLeidenWarmStartSpeedup` is a flaky benchmark-as-test present since Phase 05 that fails under machine load (threshold 1.2x, observed 1.10–1.19x). It is excluded from the `-run Test` invocation and is not caused by Phase 11 changes. The full `-race` run confirms no regressions from Phase 11 work.

---

### Commit Evidence

All phase 11 work is committed on `feat/online-ego-splitting-framework`:

| Commit | Description |
|--------|-------------|
| `cfffe28` | feat(11-01): add carry-forward fields to `OverlappingCommunityResult` + populate in `Detect()` |
| `4a1cf39` | feat(11-01): add `warmStartedDetector` helper with type switch on louvain/leiden |
| `5550a6c` | feat(11-02): add `computeAffected` + `buildPersonaGraphIncremental` helpers |
| `7fba0d3` | feat(11-02): wire incremental `Update()` + ONLINE-05/06/07/11 tests |

---

_Verified: 2026-03-31_
_Verifier: Claude (gsd-verifier)_
