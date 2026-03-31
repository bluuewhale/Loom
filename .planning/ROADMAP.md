# Roadmap: loom — Go GraphRAG Library

## Milestones

- ✅ **v1.0 Community Detection** — Phases 01-04 (shipped 2026-03-29)
- ✅ **v1.1 Online Community Detection** — Phase 05 (shipped 2026-03-30)
- ✅ **v1.2 Overlapping Community Detection** — Phases 06-09 (shipped 2026-03-31)
- **v1.3 Online Ego-Splitting** — Phases 10-13 (active)

## Phases

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

## v1.3 Online Ego-Splitting

- [x] **Phase 10: Online API Contract** — `GraphDelta` type, `Update()` signature, directed-graph guard, empty-delta fast-path (completed 2026-03-31)
- [ ] **Phase 11: Incremental Recomputation Core** — affected-node set computation, incremental ego-net rebuild, incremental persona graph patch, warm-start global detection, PersonaID collision safety
- [ ] **Phase 12: Parallel Ego-Net Construction and Performance** — goroutine pool for ego-net construction, ≥10x speedup benchmarks for 1-node and 1-edge updates, 10K-node benchmark ≤300ms/op
- [ ] **Phase 13: Correctness Hardening and Race Safety** — result invariant tests, concurrent-safe verification under `-race`

## Phase Details

### Phase 10: Online API Contract
**Goal**: Expose the public surface for incremental updates — types, method signature, guard clause, and zero-cost empty-delta fast-path — so callers can depend on a stable contract before incremental logic exists.
**Depends on**: Phase 09 (existing `EgoSplittingDetector` and `OverlappingCommunityResult`)
**Requirements**: ONLINE-01, ONLINE-02, ONLINE-03, ONLINE-04
**Estimated plans**: 1
**Success Criteria** (what must be TRUE):
  1. Caller can construct a `GraphDelta{AddedNodes: ..., AddedEdges: ...}` value and pass it to `Update()` without compilation errors
  2. `Update()` called with an empty `GraphDelta` returns the prior result pointer-equal to input within a single allocation, verified by benchmark showing O(1) behavior
  3. `Update()` called on a directed graph returns `ErrDirectedNotSupported` and no result, matching the behavior of `Detect()` on directed graphs
**Plans**: 1 plan
Plans:
- [x] 10-01-PLAN.md — GraphDelta type, OnlineOverlappingCommunityDetector interface, Update() with guards and empty-delta fast-path

### Phase 11: Incremental Recomputation Core
**Goal**: Replace the full-graph recompute path inside `Update()` with incremental logic — affected-node scoping, ego-net selective rebuild, persona graph patching, and warm-started global detection — while preserving PersonaID disjointness.
**Depends on**: Phase 10
**Requirements**: ONLINE-05, ONLINE-06, ONLINE-07, ONLINE-11
**Estimated plans**: 2
**Success Criteria** (what must be TRUE):
  1. Adding 1 node to a 34-node graph triggers ego-net recomputation for that node and its neighbors only — verified by instrumenting rebuild count in a test
  2. Personas for unaffected nodes carry over from the prior result unchanged — verified by asserting persona ID stability for untouched nodes in a table-driven test
  3. New personas are allocated from `maxExistingPersonaID + 1`, confirmed by asserting no overlap between original `NodeID` space and post-update persona IDs across all nodes in the result
**Plans**: 2 plans
Plans:
- [ ] 11-01-PLAN.md — Carry-forward fields on OverlappingCommunityResult + warmStartedDetector helper
- [ ] 11-02-PLAN.md — Incremental Update() with computeAffected, buildPersonaGraphIncremental, warm-start global detection

### Phase 12: Parallel Ego-Net Construction and Performance
**Goal**: Introduce a goroutine pool for ego-net construction so that both the incremental path and the full `Detect()` path meet their performance targets on large graphs.
**Depends on**: Phase 11
**Requirements**: ONLINE-08, ONLINE-09, ONLINE-10
**Estimated plans**: 2
**Success Criteria** (what must be TRUE):
  1. `BenchmarkUpdate1Node` reports ns/op ≥10x lower than `BenchmarkDetect` run on the same post-addition graph (Karate Club + 1 node), measured with `go test -bench`
  2. `BenchmarkUpdate1Edge` reports ns/op ≥10x lower than `BenchmarkDetect` run on the same post-addition graph (Karate Club + 1 edge), measured with `go test -bench`
  3. `BenchmarkEgoSplitting10K` reports ≤300ms/op on a 10,000-node graph, down from the ~1500ms/op baseline measured before this phase
**Plans**: TBD

### Phase 13: Correctness Hardening and Race Safety
**Goal**: Prove through tests that `Update()` results satisfy all structural invariants and that concurrent use on distinct detector instances produces no data races.
**Depends on**: Phase 12
**Requirements**: ONLINE-12, ONLINE-13
**Estimated plans**: 1
**Success Criteria** (what must be TRUE):
  1. A table-driven test covering at least: single-node addition, single-edge addition, multi-node batch addition, and an empty delta — asserts that every node in the updated graph appears in ≥1 community and that `NodeCommunities`/`Communities` are mutually consistent
  2. `go test -race ./graph/...` passes with zero race reports on a test that launches N goroutines each calling `Update()` concurrently on their own `EgoSplittingDetector` instance
**Plans**: TBD

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
| 10: Online API Contract | v1.3 | 0/1 | Planned | - |
| 11: Incremental Recomputation Core | v1.3 | 0/2 | Not started | - |
| 12: Parallel Ego-Net Construction and Performance | v1.3 | 0/2 | Not started | - |
| 13: Correctness Hardening and Race Safety | v1.3 | 0/1 | Not started | - |
