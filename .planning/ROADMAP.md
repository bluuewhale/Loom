# Roadmap: loom — Community Detection (Milestone 1)

## Overview

Phase 01 (graph data structures, modularity, registry) is complete. This roadmap covers the remaining work to ship a production-ready community detection library: a swappable `CommunityDetector` interface with Louvain and Leiden implementations, validated against standard benchmark graphs, and meeting the <100ms / 10K-node performance target.

## Phases

**Phase Numbering:** Continuing from Phase 01 (complete). New phases start at 02.

- [x] **Phase 01: Graph Data Structures, Modularity, Registry** - Weighted graph, modularity Q, NodeRegistry — COMPLETE
- [ ] **Phase 02: Interface + Louvain Core** - CommunityDetector interface, LouvainDetector with all edge-case guards
- [ ] **Phase 03: Leiden Implementation** - LeidenDetector with refinement phase and connected-community guarantee
- [ ] **Phase 04: Performance Hardening + Benchmark Fixtures** - <100ms target verified, Football/Polbooks fixtures, race-free

## Phase Details

### Phase 01: Graph Data Structures, Modularity, Registry
**Goal**: Core graph primitives and quality metric are in place
**Depends on**: Nothing
**Requirements**: GRAPH-01, GRAPH-02, MOD-01, MOD-02, REG-01, FIXT-01
**Success Criteria** (what must be TRUE):
  1. Weighted directed/undirected graph builds and queries correctly
  2. Newman-Girvan modularity Q matches Karate Club ground truth (Q ≈ 0.371)
  3. NodeRegistry round-trips string labels to NodeID and back
**Plans**: 3 plans

Plans:
- [x] 01-01: Graph data structure
- [x] 01-02: Modularity + Karate Club fixture
- [x] 01-03: NodeRegistry

### Phase 02: Interface + Louvain Core
**Goal**: Callers can run community detection via a swappable interface; Louvain produces correct partitions on all inputs including edge cases
**Depends on**: Phase 01
**Requirements**: IFACE-01, IFACE-02, IFACE-03, IFACE-04, IFACE-05, IFACE-06, LOUV-01, LOUV-02, LOUV-03, LOUV-04, LOUV-05
**Success Criteria** (what must be TRUE):
  1. `NewLouvain(opts)` and `NewLeiden(opts)` both satisfy `CommunityDetector` — swap-in works with a single variable change
  2. `Detect(g)` on Karate Club returns Q > 0.35 and a valid 2-4 community partition
  3. `Detect(g)` on empty graph, single node, two-node graph, and fully disconnected graph all return without panic or error
  4. `CommunityResult` exposes `Partition`, `Modularity`, `Passes`, and `Moves` fields populated with non-zero data
**Plans**: 2 plans

Plans:
- [x] 02-01: detector.go (CommunityDetector interface, CommunityResult, LouvainOptions, LeidenOptions)
- [ ] 02-02: louvain.go + louvain_state.go (Phase 1 local move, Phase 2 supergraph, convergence, edge-case guards)

### Phase 03: Leiden Implementation
**Goal**: `LeidenDetector` produces connected communities with NMI accuracy equal to or better than Louvain on standard graphs
**Depends on**: Phase 02
**Requirements**: LEID-01, LEID-02, LEID-03, LEID-04
**Success Criteria** (what must be TRUE):
  1. `NewLeiden(opts).Detect(karateClub)` returns Q > 0.35 and NMI >= 0.7 vs ground-truth partition
  2. Every community in every Leiden result is internally connected (no disconnected communities)
  3. Leiden and Louvain are drop-in swaps — identical call site, `CommunityDetector` variable only changes constructor
**Plans**: 1 plan

Plans:
- [ ] 03-01: leiden.go + leiden_state.go (local move, refinement phase, aggregation)

### Phase 04: Performance Hardening + Benchmark Fixtures
**Goal**: Both algorithms meet the <100ms / 10K-node target, concurrent use is race-free, and accuracy is validated on three benchmark graphs
**Depends on**: Phase 03
**Requirements**: PERF-01, PERF-02, PERF-03, PERF-04, TEST-01, TEST-02, TEST-03, TEST-04, TEST-05
**Success Criteria** (what must be TRUE):
  1. `go test -bench=BenchmarkLouvain10K -benchmem` and `BenchmarkLeiden10K` both report < 100ms/op
  2. `go test -race ./graph/...` passes with zero data race reports
  3. Louvain and Leiden both achieve Q > 0.35 on Karate Club, and NMI validation passes on Football (115 nodes) and Polbooks (105 nodes) fixtures
  4. All 8 edge cases (empty, single node, disconnected, giant+singletons, two-node, zero resolution, complete graph, self-loop) pass without error
  5. `sync.Pool` warmup shows 0 allocs/op on repeated same-size graph calls (`-benchmem`)
**Plans**: 2 plans

Plans:
- [ ] 04-01: Football + Polbooks fixtures, NMI helper, accuracy test suite
- [ ] 04-02: sync.Pool integration, neighborWeightBuf dirty-list, benchmarks + benchstat baseline

## Progress

**Execution Order:** 01 (done) → 02 → 03 → 04

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 01. Graph Data Structures, Modularity, Registry | 3/3 | Complete | 2026-03-29 |
| 02. Interface + Louvain Core | 1/2 | In Progress|  |
| 03. Leiden Implementation | 0/1 | Not started | - |
| 04. Performance Hardening + Benchmark Fixtures | 0/2 | Not started | - |
