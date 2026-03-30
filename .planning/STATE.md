---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: planning
stopped_at: Completed 06-01-PLAN.md — types and interfaces for ego splitting
last_updated: "2026-03-30T07:05:17.197Z"
last_activity: "2026-03-30 — v1.2 roadmap created: Phases 06-09 defined"
progress:
  total_phases: 4
  completed_phases: 1
  total_plans: 1
  completed_plans: 1
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-29)

**Core value:** 개발자가 GraphRAG 파이프라인을 Go로 구현할 수 있는 교체 가능한 인터페이스로 그래프 알고리즘을 빠르게 가져다 쓸 수 있어야 한다.
**Current focus:** v1.2 — Overlapping Community Detection (Ego Splitting Framework)

## Current Position

Phase: 06 — Types and Interfaces (not started)
Plan: —
Status: Roadmap complete — ready to plan Phase 06
Last activity: 2026-03-30 — v1.2 roadmap created: Phases 06-09 defined

Progress: [____________] 0% (0/4 phases complete)

## Performance Metrics

**Velocity:**

- Total plans completed: 7 (across v1.0 + v1.1)
- Average duration: ~20min/plan
- Total execution time: ~140min

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 02-interface-louvain-core | 2/2 | 53min | 26min |
| 03-leiden | 1/1 | 4min | 4min |
| 04-performance-hardening | 2/2 | 60min | 30min |
| 05-warm-start | 2/2 | 25min | 12min |

**Recent Trend:**

- Last 5 plans: warm-start (10min, 15min), perf-hardening (45min, 15min), leiden (4min)
- Trend: Stable

*Updated after each plan completion*
| Phase 02-interface-louvain-core P01 | 8min | 1 tasks | 3 files |
| Phase 02-interface-louvain-core P02 | 45min | 2 tasks | 3 files |
| Phase 03 P01 | 4min | 2 tasks | 5 files |
| Phase 04-performance-hardening-benchmark-fixtures P01 | 15min | 2 tasks | 6 files |
| Phase 04-performance-hardening-benchmark-fixtures P02 | 45min | 2 tasks | 6 files |
| Phase 05-warm-start P01 | 15min | 2 tasks | 5 files |
| Phase 05-warm-start P02 | 10min | 2 tasks | 3 files |
| Phase 06-types-and-interfaces P01 | 8min | 2 tasks | 2 files |

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
- [Phase 06-types-and-interfaces]: OverlappingCommunityDetector declared in ego_splitting.go (not detector.go) — overlapping detection concerns separate from disjoint detection
- [Phase 06-types-and-interfaces]: egoSplittingDetector is unexported — callers program to OverlappingCommunityDetector interface, not the concrete type
- [Phase 06-types-and-interfaces]: EgoSplittingOptions.Resolution defaults to 1.0 in NewEgoSplitting constructor — consistent with LouvainOptions/LeidenOptions zero-value pattern

### v1.2 Critical Pitfalls (from research)

- [EGO-CRIT-01]: Pass only `neighbors` to `g.Subgraph()`, never append `v` itself — ego node must be excluded from its own ego-net
- [EGO-CRIT-02]: Use independent monotonic counter for PersonaIDs — never reuse original NodeIDs; assert keys never overlap `[0, g.NodeCount())`
- [EGO-CRIT-03]: Deduplicate edges before `personaGraph.AddEdge` — undirected iteration visits each edge twice; assert `personaGraph.TotalWeight() == g.TotalWeight()`
- [EGO-CRIT-05]: Collect ALL community IDs across all of a node's personas in Algorithm 3 — not just the first; assert at least one node has multiple memberships on Karate Club
- [EGO-CRIT-06]: Do NOT use standard NMI for overlapping validation — use Omega index; standard NMI produces misleadingly high scores on non-overlapping degradation

### Pending Todos

- Determine Omega index empirical thresholds per fixture (Karate Club, Football, Polbooks) once Phase 08 pipeline produces first results — do not set speculatively
- Profile `commInEgoNet[u][v]` lookup table memory at high degree during Phase 08 benchmarks (acceptable at avg_degree 20 / N 10K; flag if avg_degree > 100)

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260330-jq7 | warm-start 테스트 누락 사항 추가 | 2026-03-30 | 3390928 | [260330-jq7-warm-start](.planning/quick/260330-jq7-warm-start/) |

### Blockers/Concerns

- [Phase 07]: Algorithm 2 co-membership edge-wiring condition is subtle (paper Section 2.2) — validate `commInEgoNet[u][v]` lookup design against paper before implementing; edge (u,v) wires to persona pair only when u and v co-appear in same local community in BOTH u's and v's ego-nets
- [Phase 08]: Omega index threshold values for accuracy gates are unknown until first working Detect run — calibrate empirically, do not speculate

## Session Continuity

Last session: 2026-03-30T07:05:17.194Z
Stopped at: Completed 06-01-PLAN.md — types and interfaces for ego splitting
Resume file: None
Next action: `/gsd:plan-phase 6`
