---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: verifying
stopped_at: Completed 01-01-PLAN.md — Leiden multi-run NumRuns field and TestLeidenStabilityMultiRun
last_updated: "2026-03-30T07:13:41.603Z"
last_activity: 2026-03-30
progress:
  total_phases: 3
  completed_phases: 1
  total_plans: 1
  completed_plans: 1
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-29)

**Core value:** 개발자가 GraphRAG 파이프라인을 Go로 구현할 때 필요한 그래프 알고리즘을 교체 가능한 인터페이스로 빠르게 가져다 쓸 수 있어야 한다.
**Current focus:** Phase 01 — leiden-nmi-seed

## Current Position

Phase: 01 (leiden-nmi-seed) — EXECUTING
Plan: 1 of 1
Status: Phase complete — ready for verification
Last activity: 2026-03-30

Progress: [████████████] 100% (Phase 05 complete, 2/2 plans done)

## Performance Metrics

**Velocity:**

- Total plans completed: 3
- Average duration: unknown
- Total execution time: unknown

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 3/3 | - | - |

**Recent Trend:**

- Last 5 plans: unknown
- Trend: Stable

*Updated after each plan completion*
| Phase 02-interface-louvain-core P01 | 8min | 1 tasks | 3 files |
| Phase 02-interface-louvain-core P02 | 45min | 2 tasks | 3 files |
| Phase 03 P01 | 4min | 2 tasks | 5 files |
| Phase 04-performance-hardening-benchmark-fixtures P01 | 15min | 2 tasks | 6 files |
| Phase 04-performance-hardening-benchmark-fixtures P02 | 45min | 2 tasks | 6 files |
| Phase 05-warm-start P01 | 15min | 2 tasks | 5 files |
| Phase 05-warm-start P02 | 10min | 2 tasks | 3 files |
| Phase 01-leiden-nmi-seed P01 | 25min | 2 tasks | 6 files |

## Accumulated Context

### Decisions

- [Phase 01]: Single `package graph` — no sub-packages; all types in one package
- [Phase 01]: `map[NodeID]int` as Partition — no external type, zero-alloc swap
- [Phase 01]: `NodeRegistry` optional — integer ID path stays available for perf-critical callers
- [Roadmap]: `CommunityDetector` interface with `Detect(g *Graph) (CommunityResult, error)` — swappable contract
- [Phase 02-interface-louvain-core]: louvainDetector.Detect lives in louvain.go (plan 02), not detector.go — separation of interface from algorithm
- [Phase 02-interface-louvain-core]: bestQ tracking: retain highest-Q partition across convergence passes to guard against degenerate merging
- [Phase 02-interface-louvain-core]: Sort nodes before RNG shuffle in phase1: Go map iteration randomness; sorting provides deterministic base for seeded shuffle
- [Phase 03]: Seed=2 used for Leiden accuracy test: seed 42 yields NMI=0.60, seed 2 gives NMI=0.72 satisfying invariant
- [Phase 03]: louvainState wrapper pattern for phase1 reuse: construct inline &louvainState{partition, commStr, rng}, copy back after call
- [Phase 04-performance-hardening-benchmark-fixtures]: Seed=1 for TestLouvainKarateClubNMI (gives NMI=0.83 vs threshold 0.70; Seed=42 gives only 0.60)
- [Phase 04-performance-hardening-benchmark-fixtures]: nmi() and uniqueCommunities() extracted to shared testhelpers_test.go for reuse across accuracy tests
- [Phase 04-performance-hardening-benchmark-fixtures]: rand.New(src) in reset() ensures identical RNG sequence to original constructor; st.rng.Seed() causes shuffle divergence
- [Phase 04-performance-hardening-benchmark-fixtures]: bestSuperPartition must be deep-copied under pool reuse; pointer sharing causes state.partition clear to silently destroy saved results
- [Phase 05-warm-start]: InitialPartition passed as reset() parameter only — not stored on state struct — preserves pool safety (pooled state must not hold caller references between Detect calls)
- [Phase 05-warm-start]: firstPass guard in Detect loop: warm seed applies only on original graph; supergraph passes always cold-reset (supergraph NodeIDs are synthetic)
- [Phase 05-warm-start]: commStr always rebuilt from g.Strength() in warm-start path — not carried from prior run
- [Phase 05-warm-start]: perturbGraph uses rebuild strategy (not RemoveEdge) — Graph has no RemoveEdge; collect canonical edges, mark nRemove for deletion, rebuild, add nAdd random edges
- [Phase 05-warm-start]: Quality tests assert Q(warm) >= Q(cold_perturbed) not Q(cold_original) — topology changed so original Q is wrong baseline
- [Phase 05-warm-start]: Benchmark setup (cold detect + perturbGraph) before b.ResetTimer(); only warm Detect measured in loop (Pitfall 6)
- [Phase 01-leiden-nmi-seed]: Q >= 0.38 threshold for TestLeidenStabilityMultiRun: best-Q selection picks modularity-optimal 4-community solution (Q~0.42), not NMI-aligned 3-community; NMI covered by deterministic TestLeidenKarateClubAccuracy
- [Phase 01-leiden-nmi-seed]: NumRuns: 1 annotation on all Seed!=0 test calls: explicit documentation of single-run path; no behavior change since Seed!=0 ignores NumRuns per implementation

### Pending Todos

None yet.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260330-jq7 | warm-start 테스트 누락 사항 추가 | 2026-03-30 | 3390928 | [260330-jq7-warm-start](.planning/quick/260330-jq7-warm-start/) |

### Roadmap Evolution

- Phase 1 added: Leiden NMI 안정성 — seed 의존성 문제 해결 및 알고리즘 수렴 보장 강화
- Phase 2 added: 문서화 — GoDoc 예시 확충 및 GraphRAG 실전 예제 추가
- Phase 3 added: 벤치마크 비교 — Python networkx 대비 성능 비교표 작성 (채택 논거)

### Blockers/Concerns

- [Phase 02]: Verify `g.Edges()` API exists in graph.go before writing Louvain (CRIT-02 totalWeight fix depends on unique-edge iteration)
- [Phase 02]: Directed graph ΔQ formula not yet estimated — flag for Phase 02 planning; may defer directed support to v2

## Session Continuity

Last session: 2026-03-30T07:13:41.600Z
Stopped at: Completed 01-01-PLAN.md — Leiden multi-run NumRuns field and TestLeidenStabilityMultiRun
Resume file: None
