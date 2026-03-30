# Roadmap: loom — Go GraphRAG Library

## Milestones

- ✅ **v1.0 Community Detection** — Phases 01-04 (shipped 2026-03-29)
- ✅ **v1.1 Online Community Detection** — Phase 05 (shipped 2026-03-30)

## Phases

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

### Phase 1: Leiden NMI 안정성 — seed 의존성 문제 해결 및 알고리즘 수렴 보장 강화

**Goal:** LeidenOptions에 NumRuns 멀티런 전략을 추가하여 Seed=0 모드에서 NMI 품질 안정성 확보
**Requirements**: LEIDEN-NMI-01, LEIDEN-NMI-02, LEIDEN-NMI-03
**Depends on:** Phase 0
**Plans:** 1 plan

Plans:
- [x] 01-01-PLAN.md — NumRuns multi-run 구현 + 테스트 업데이트 및 stability 테스트 추가

### Phase 2: 문서화 — GoDoc 예시 확충 및 GraphRAG 실전 예제 추가

**Goal:** GoDoc Example 함수 3개 추가, README에 GraphRAG 실전 예제 섹션 및 Accuracy NMI 값 기입, NumRuns API 문서화
**Requirements**: DOC-01, DOC-02, DOC-03, DOC-04, DOC-05
**Depends on:** Phase 1
**Plans:** 1 plan

Plans:
- [ ] 02-01-PLAN.md — GoDoc examples + README updates (GraphRAG example, NMI values, NumRuns, API corrections)

### Phase 3: 벤치마크 비교 — Python networkx 대비 성능 비교표 작성 (채택 논거)

**Goal:** [To be planned]
**Requirements**: TBD
**Depends on:** Phase 2
**Plans:** 0 plans

Plans:
- [ ] TBD (run /gsd:plan-phase 3 to break down)
