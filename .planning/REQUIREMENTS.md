# Requirements: loom v1.3 — Online Ego-Splitting

**Milestone:** v1.3
**Status:** Active
**Last updated:** 2026-03-31

---

## Milestone v1.3 Requirements

### Online API Contract

- [x] **ONLINE-01**: Caller can construct a `GraphDelta` value describing node/edge additions: `AddedNodes []NodeID` and `AddedEdges []Edge`
- [x] **ONLINE-02**: Caller can invoke `Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)` on an `EgoSplittingDetector` to obtain an updated overlapping community result without full recomputation
- [x] **ONLINE-03**: Caller receives prior result unchanged (no recomputation) when `Update()` is called with an empty delta (`len(AddedNodes)==0 && len(AddedEdges)==0`)
- [x] **ONLINE-04**: Caller receives `ErrDirectedNotSupported` when `Update()` is called on a directed graph, matching the existing `Detect()` guard contract

### Incremental Recomputation

- [x] **ONLINE-05**: `Update()` recomputes ego-nets only for the set of affected nodes: new nodes plus all neighbors of edge endpoints — not all `n` ego-nets in the graph
- [x] **ONLINE-06**: `Update()` patches the persona graph incrementally — only personas of affected nodes are rebuilt; unaffected nodes' personas are carried over from the prior state
- [x] **ONLINE-07**: `Update()` warm-starts the global detection phase from the prior result's community partition, reusing the `InitialPartition` field on `GlobalDetector` options (v1.1 warm-start mechanism)

### Performance

- [x] **ONLINE-08**: `BenchmarkUpdate1Node` demonstrates that `Update()` with 1 added node runs ≥10x faster than `Detect()` on the same updated graph (baseline: Karate Club 34-node graph + 1 new node)
- [x] **ONLINE-09**: `BenchmarkUpdate1Edge` demonstrates that `Update()` with 1 added edge runs ≥10x faster than `Detect()` on the same updated graph (baseline: Karate Club + 1 new edge between existing nodes)
- [x] **ONLINE-10**: Parallel ego-net construction via goroutine pool reduces `BenchmarkEgoSplitting10K` from ~1500ms/op to ≤300ms/op (resolves v1.2 deferred performance gap)

### Correctness and Safety

- [x] **ONLINE-11**: PersonaID allocation in `Update()` never collides with original `NodeID` space — new personas assigned from `maxExistingPersonaID + 1`, preserving the disjoint PersonaID invariant
- [ ] **ONLINE-12**: `Update()` result satisfies all existing result invariants: every original node (including newly added ones) appears in at least one community; `NodeCommunities` and `Communities` are mutually consistent
- [ ] **ONLINE-13**: `Update()` is concurrent-safe — `go test -race` passes on concurrent `Update()` calls on distinct detector instances

---

## Future Requirements (deferred)

- Node/edge deletions in online mode — delta-minus path requires different affected-node logic; defer to v1.4
- Overlapping NMI (McDaid 2011 / `NMI_max`) accuracy metric — more complex than Omega index; defer to v1.4
- LFR synthetic benchmark fixtures with known overlapping ground truth — defer to v1.4
- Streaming delta queue (multiple deltas applied sequentially without full recomputation chain) — defer to v1.4
- `MaxPersonasPerNode` cap for star-topology graphs in incremental path — defer until empirical profiling shows need

---

## Out of Scope

- Node/edge deletions — only additions supported in v1.3
- Modifying existing `Detect()` behavior — `Update()` is purely additive; zero breaking changes to v1.2 API
- External dependencies — stdlib only; no new imports
- Directed graph incremental support — undirected only, same constraint as `Detect()`
- Persistence of delta history — stateless Update(); prior result is the only state the caller must retain

---

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| ONLINE-01 | Phase 10 | Planned |
| ONLINE-02 | Phase 10 | Planned |
| ONLINE-03 | Phase 10 | Planned |
| ONLINE-04 | Phase 10 | Planned |
| ONLINE-05 | Phase 11 | Planned |
| ONLINE-06 | Phase 11 | Planned |
| ONLINE-07 | Phase 11 | Planned |
| ONLINE-11 | Phase 11 | Planned |
| ONLINE-08 | Phase 12 | Planned |
| ONLINE-09 | Phase 12 | Planned |
| ONLINE-10 | Phase 12 | Planned |
| ONLINE-12 | Phase 13 | Planned |
| ONLINE-13 | Phase 13 | Planned |
