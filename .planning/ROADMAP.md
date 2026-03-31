# Roadmap: loom — Go GraphRAG Library

## Milestones

- ✅ **v1.0 Community Detection** — Phases 01-04 (shipped 2026-03-29)
- ✅ **v1.1 Online Community Detection** — Phase 05 (shipped 2026-03-30)
- ✅ **v1.2 Overlapping Community Detection** — Phases 06-09 (shipped 2026-03-31)
- ✅ **v1.3 Online Ego-Splitting** — Phases 10-13 (shipped 2026-03-31)

## Phases

<details>
<summary>✅ v1.3 Online Ego-Splitting (Phases 10-13) — SHIPPED 2026-03-31</summary>

- [x] Phase 10: Online API Contract (1/1 plan) — completed 2026-03-31
- [x] Phase 11: Incremental Recomputation Core (2/2 plans) — completed 2026-03-31
- [x] Phase 12: Parallel Ego-Net Construction and Performance (2/2 plans) — completed 2026-03-31
- [x] Phase 13: Correctness Hardening and Race Safety (1/1 plan) — completed 2026-03-31

Full details: `.planning/milestones/v1.3-ROADMAP.md`

</details>

<details>
<summary>✅ v1.2 Overlapping Community Detection (Phases 06-09) — SHIPPED 2026-03-31</summary>

- [x] Phase 06: Types and Interfaces (1/1 plan) — completed 2026-03-30
- [x] Phase 07: Persona Graph Infrastructure (2/2 plans) — completed 2026-03-30
- [x] Phase 08: Full Detect Pipeline + Accuracy + Performance (2/2 plans) — completed 2026-03-30
- [x] Phase 09: Edge Cases and Hardening (1/1 plan) — completed 2026-03-30

Full details: `.planning/milestones/v1.2-ROADMAP.md`

</details>

<details>
<summary>✅ v1.1 Online Community Detection (Phase 05) — SHIPPED 2026-03-30</summary>

- [x] Phase 05: Warm Start — Online Community Detection (2/2 plans) — completed 2026-03-30

Full details: `.planning/milestones/v1.1-ROADMAP.md`

</details>

<details>
<summary>✅ v1.0 Community Detection (Phases 01-04) — SHIPPED 2026-03-29</summary>

- [x] Phase 01: Graph Data Structures, Modularity, Registry (pre-planned) — completed 2026-03-29
- [x] Phase 02: Interface + Louvain Core (2/2 plans) — completed 2026-03-29
- [x] Phase 03: Leiden Implementation (1/1 plan) — completed 2026-03-29
- [x] Phase 04: Performance Hardening + Benchmark Fixtures (2/2 plans) — completed 2026-03-29

Full details: `.planning/milestones/v1.0-ROADMAP.md`

</details>

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 01: Graph Data Structures | v1.0 | pre-planned | Complete | 2026-03-29 |
| 02: Interface + Louvain Core | v1.0 | 2/2 | Complete | 2026-03-29 |
| 03: Leiden Implementation | v1.0 | 1/1 | Complete | 2026-03-29 |
| 04: Performance Hardening | v1.0 | 2/2 | Complete | 2026-03-29 |
| 05: Warm Start | v1.1 | 2/2 | Complete | 2026-03-30 |
| 06: Types and Interfaces | v1.2 | 1/1 | Complete | 2026-03-30 |
| 07: Persona Graph Infrastructure | v1.2 | 2/2 | Complete | 2026-03-30 |
| 08: Full Detect Pipeline + Accuracy + Performance | v1.2 | 2/2 | Complete | 2026-03-30 |
| 09: Edge Cases and Hardening | v1.2 | 1/1 | Complete | 2026-03-30 |
| 10: Online API Contract | v1.3 | 1/1 | Complete | 2026-03-31 |
| 11: Incremental Recomputation Core | v1.3 | 2/2 | Complete | 2026-03-31 |
| 12: Parallel Ego-Net Construction and Performance | v1.3 | 2/2 | Complete | 2026-03-31 |
| 13: Correctness Hardening and Race Safety | v1.3 | 1/1 | Complete | 2026-03-31 |

### Phase 14: reset() warm-start 최적화 — Louvain·Leiden commStr 전체 재계산 제거, affected 노드만 delta 패치

**Goal:** Eliminate sorted-node-slice and commStr full-rebuild bottlenecks in louvainState.reset() and leidenState.reset() warm-start paths; reduce BenchmarkEgoSplittingUpdate1Node1Edge from ~175ms/op to ≤150ms/op.
**Requirements**: RESET-OPT-01, RESET-OPT-02
**Depends on:** Phase 13
**Plans:** 1 plan

Plans:
- [ ] 14-01-PLAN.md — Sorted-node cache + commStr delta patch in louvainState and leidenState; regression guard test
