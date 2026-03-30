# Roadmap: loom — Go GraphRAG Library

## Milestones

- ✅ **v1.0 Community Detection** — Phases 01-04 (shipped 2026-03-29)
- ✅ **v1.1 Online Community Detection** — Phase 05 (shipped 2026-03-30)
- 🔄 **v1.2 Overlapping Community Detection** — Phases 06-09 (active)

## Phases

<details>
<summary>🔄 v1.2 Overlapping Community Detection (Phases 06-09) — ACTIVE</summary>

- [x] **Phase 06: Types and Interfaces** - Define `OverlappingCommunityDetector`, `OverlappingCommunityResult`, `EgoSplittingOptions`, and `NewEgoSplitting` stub
- [ ] **Phase 07: Persona Graph Infrastructure** - Implement `personaMap`, `buildPersonaGraph`, `mapPersonasToOriginal`, and Algorithm 1 ego-net construction
- [ ] **Phase 08: Full Detect Pipeline + Accuracy + Performance** - Wire Algorithms 1-3 into `Detect`, integrate Omega index, validate fixtures, benchmark 10K
- [ ] **Phase 09: Edge Cases and Hardening** - Guards for empty graph, isolated nodes, single-community ego-nets, and degenerate inputs

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

## Phase Details

### Phase 06: Types and Interfaces
**Goal**: Callers can reference the `OverlappingCommunityDetector` interface and its result/options types; the package compiles and all existing tests continue to pass
**Depends on**: Phase 05 (complete)
**Requirements**: EGO-01, EGO-02, EGO-03, EGO-07
**Success Criteria** (what must be TRUE):
  1. `OverlappingCommunityDetector` interface with `Detect(*Graph) (OverlappingCommunityResult, error)` is declared and distinct from `CommunityDetector` — no existing file is modified
  2. `OverlappingCommunityResult` exposes `Communities [][]NodeID` and `NodeCommunities map[NodeID][]int` on the same result value
  3. `EgoSplittingOptions` accepts `LocalDetector CommunityDetector`, `GlobalDetector CommunityDetector`, and `Resolution float64`; nil detectors default to Louvain
  4. `NewEgoSplitting(opts EgoSplittingOptions)` returns a value that satisfies `OverlappingCommunityDetector`; calling `Detect` returns a defined unimplemented sentinel error
  5. `go build ./...` and `go test ./...` pass with zero failures (stub, no logic yet)
**Plans**: 1 plan
- [x] 06-01-PLAN.md — Types, stub constructor, and tests
**UI hint**: no

### Phase 07: Persona Graph Infrastructure
**Goal**: Algorithm 1 (ego-net construction) and Algorithm 2 (persona graph generation) are implemented and validated in isolation on hand-crafted small graphs before being wired into the full pipeline
**Depends on**: Phase 06
**Requirements**: EGO-04, EGO-05, EGO-06
**Success Criteria** (what must be TRUE):
  1. For every node v, the ego-net subgraph passed to `LocalDetector.Detect` contains no node equal to v (ego node excluded per paper definition)
  2. Every PersonaID in the persona graph is strictly outside `[0, g.NodeCount())` — no collision with original NodeIDs
  3. `personaGraph.TotalWeight()` equals `g.TotalWeight()` after `buildPersonaGraph` — no edge double-counting
  4. `mapPersonasToOriginal` returns a bijective inverse map: every persona ID maps to exactly one original NodeID, with no unmapped personas
  5. Running `GlobalDetector.Detect` on the persona graph and applying `mapPersonasToOriginal` produces an `OverlappingCommunityResult` where at least one original node holds memberships from more than one persona (verified on Karate Club)
**Plans**: 2 plans
- [ ] 07-01-PLAN.md — buildEgoNet, buildPersonaGraph, mapPersonasToOriginal with triangle/barbell tests
- [ ] 07-02-PLAN.md — Karate Club integration test for Algorithm 1+2+3 flow
**UI hint**: no

### Phase 08: Full Detect Pipeline + Accuracy + Performance
**Goal**: `EgoSplittingDetector.Detect` is end-to-end correct, concurrent-safe, achieves Omega index >= 0.5 on all three fixture graphs, and completes within 300ms on a 10,000-node graph
**Depends on**: Phase 07
**Requirements**: EGO-08, EGO-09, EGO-10, EGO-11
**Success Criteria** (what must be TRUE):
  1. `OmegaIndex(result, groundTruth)` is callable and returns a float64 in [0, 1] — standard NMI is not used as the accuracy gate
  2. `EgoSplittingDetector.Detect` achieves Omega index >= 0.5 on Karate Club (34n), Football (115n), and Polbooks (105n) ground-truth fixtures
  3. `go test -race ./...` passes with zero race reports on the full Detect pipeline
  4. `BenchmarkEgoSplitting10K` completes in <= 300ms/op on a 10,000-node synthetic graph
**Plans**: 1 plan
- [ ] 06-01-PLAN.md — Types, stub constructor, and tests
**UI hint**: no

### Phase 09: Edge Cases and Hardening
**Goal**: `EgoSplittingDetector.Detect` handles all degenerate inputs without panicking and returns defined results or errors in every documented edge case
**Depends on**: Phase 08
**Requirements**: EGO-12, EGO-13, EGO-14
**Success Criteria** (what must be TRUE):
  1. Degree-0 (isolated) nodes are assigned to their own singleton community — `Detect` does not panic and every isolated node appears in exactly one community
  2. A node whose ego-net yields a single local community produces exactly one persona equal to the original node (no splitting) — persona count does not grow unboundedly on star topologies
  3. `Detect` called on an empty graph (`NodeCount == 0`) returns a non-nil sentinel error and a zero-value result
**Plans**: 1 plan
- [ ] 06-01-PLAN.md — Types, stub constructor, and tests
**UI hint**: no

## Progress

| Phase | Milestone | Plans | Status | Completed |
|-------|-----------|-------|--------|-----------|
| 01: Graph Data Structures | v1.0 | pre-planned | Complete | 2026-03-29 |
| 02: Interface + Louvain Core | v1.0 | 2/2 | Complete | 2026-03-29 |
| 03: Leiden Implementation | v1.0 | 1/1 | Complete | 2026-03-29 |
| 04: Performance Hardening | v1.0 | 2/2 | Complete | 2026-03-29 |
| 05: Warm Start | v1.1 | 2/2 | Complete | 2026-03-30 |
| 06: Types and Interfaces | v1.2 | 0/1 | Planned | - |
| 07: Persona Graph Infrastructure | v1.2 | 0/2 | Planned | - |
| 08: Full Detect Pipeline + Accuracy + Performance | v1.2 | 0/? | Not started | - |
| 09: Edge Cases and Hardening | v1.2 | 0/? | Not started | - |
