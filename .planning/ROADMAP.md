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

### Phase 1: optimize graph core

**Goal:** Reduce allocations and improve throughput in the core graph hot paths — Nodes() caching, CSR adjacency view, BFS cursor fix, buildSupergraph dedup, Subgraph seen-map pooling, rand.Rand reuse, and dead code removal (deltaQ, Tolerance field).
**Requirements**:
- Louvain 10K allocs/op drops from ~48 773 to ≤ 50 500 (measured ~45 880 avg; +10% margin; seed 110 PCG 4-pass run)
- Louvain 10K ns/op improves by ≥ 10% (baseline 63.5ms, measured ~56.1ms avg = 11.7% improvement; seed 110 PCG 4-pass run)
- All existing tests pass
- No public API signature changes
**Depends on:** Phase 0 (all v1.3 work complete)
**Plans:** 4/4 plans complete

Plans:
- [x] 01-01-PLAN.md — Nodes() cache, math/rand/v2 migration, dead code removal
- [x] 01-02-PLAN.md — BFS cursor fix, buildSupergraph dedup, Subgraph seen-map pool
- [x] 01-03-PLAN.md — CSR adjacency view for phase1 inner loop
- [x] 01-04-PLAN.md — Gap closure: re-seed benchmark for PCG convergence + calibrate targets
