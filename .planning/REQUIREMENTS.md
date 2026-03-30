# Requirements: loom v1.2 — Overlapping Community Detection (Ego Splitting Framework)

**Milestone:** v1.2
**Status:** Active
**Last updated:** 2026-03-30

---

## Milestone v1.2 Requirements

### Core API

- [ ] **EGO-01**: Caller can use `OverlappingCommunityDetector` interface with `Detect(g *Graph) (OverlappingCommunityResult, error)` — distinct from existing `CommunityDetector`, zero breaking changes
- [ ] **EGO-02**: Caller can access overlapping result as `Communities [][]NodeID` (community-first) and `NodeCommunities map[NodeID][]int` (node-first O(1) lookup) from `OverlappingCommunityResult`
- [ ] **EGO-03**: Caller can configure `EgoSplittingOptions` with `LocalDetector CommunityDetector`, `GlobalDetector CommunityDetector`, and `Resolution float64` — both detectors default to Louvain if nil

### Algorithm Implementation

- [ ] **EGO-04**: Caller can construct ego-net for each node u as `G[N(u)]` (neighbors only, u excluded) via Algorithm 1 using existing `g.Subgraph()` + `LocalDetector.Detect()`
- [ ] **EGO-05**: Caller can generate persona graph where each (node, local-community) pair becomes one persona node with disjoint PersonaID space `[N, N+P)` and deduplicated edge rewiring (Algorithm 2)
- [ ] **EGO-06**: Caller can recover overlapping community membership by running `GlobalDetector.Detect()` on persona graph and mapping persona assignments back to original nodes (Algorithm 3)
- [ ] **EGO-07**: Caller can construct an `EgoSplittingDetector` via `NewEgoSplitting(opts EgoSplittingOptions)` that implements `OverlappingCommunityDetector`

### Accuracy Validation

- [ ] **EGO-08**: Caller can validate overlapping community quality using `OmegaIndex(result OverlappingCommunityResult, groundTruth [][]NodeID) float64`
- [ ] **EGO-09**: `EgoSplittingDetector` achieves Omega index ≥ 0.5 on Karate Club (34n), Football (115n), and Polbooks (105n) fixtures

### Performance and Concurrency

- [ ] **EGO-10**: `EgoSplittingDetector.Detect()` is concurrent-safe — `go test -race` passes
- [ ] **EGO-11**: `EgoSplittingDetector.Detect()` completes in ≤ 300ms on a 10,000-node graph (benchmark)

### Edge Cases

- [ ] **EGO-12**: `EgoSplittingDetector` handles degree-0 nodes (isolated nodes assigned to their own singleton community without panic)
- [ ] **EGO-13**: `EgoSplittingDetector` handles nodes whose ego-net yields a single community (persona = original node, no splitting)
- [ ] **EGO-14**: `EgoSplittingDetector` returns a defined error on empty graph input

---

## Future Requirements (deferred)

- Parallel ego-net detection via goroutine pool (defer until sequential correctness proven and benchmarks show gap)
- Overlapping NMI (McDaid 2011 / `NMI_max`) — more complex than Omega index, defer to v1.3
- Best-match F1 metric — defer to v1.3
- LFR synthetic benchmark fixtures with known overlapping ground truth — defer to v1.3
- `MaxPersonasPerNode` cap for star-topology graphs — defer until empirical profiling needed

---

## Out of Scope

- Modifying existing `CommunityDetector`, `CommunityResult`, `LouvainDetector`, `LeidenDetector` — additive only
- External dependencies — implementation uses stdlib + existing `package graph` only
- Directed graph support for ego-nets — undirected only in v1.2
- Visualization of overlapping communities — external tooling responsibility
- Persistence / serialization of `OverlappingCommunityResult` — in-memory only

---

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| EGO-01 | Phase 06 | Pending |
| EGO-02 | Phase 06 | Pending |
| EGO-03 | Phase 06 | Pending |
| EGO-04 | Phase 07 | Pending |
| EGO-05 | Phase 07 | Pending |
| EGO-06 | Phase 07 | Pending |
| EGO-07 | Phase 06 | Pending |
| EGO-08 | Phase 08 | Pending |
| EGO-09 | Phase 08 | Pending |
| EGO-10 | Phase 08 | Pending |
| EGO-11 | Phase 08 | Pending |
| EGO-12 | Phase 09 | Pending |
| EGO-13 | Phase 09 | Pending |
| EGO-14 | Phase 09 | Pending |

*Note: Traceability will be finalized by roadmapper agent.*
