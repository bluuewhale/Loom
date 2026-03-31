# Project Research Summary

**Project:** Ego Splitting Framework (v1.2)
**Domain:** Overlapping community detection — Go library extension
**Researched:** 2026-03-30
**Confidence:** HIGH

## Executive Summary

The v1.2 milestone adds overlapping community detection to the existing `package graph` Go library (which already implements Louvain and Leiden). The chosen algorithm is the Ego Splitting Framework (Epasto et al., KDD 2017), a three-step reduction that transforms the overlapping detection problem into a standard non-overlapping problem on a "persona graph," then lifts the result back. The key insight is that every node in a social graph can occupy multiple social roles (personas), each corresponding to a distinct community in its local neighborhood. By splitting nodes into personas and rewiring edges, the existing `CommunityDetector` interface (Louvain or Leiden) is reused without modification for both local ego-net detection and persona-graph detection.

The recommended implementation strategy is purely additive: zero new runtime dependencies, no changes to existing files, and four new source files within `package graph`. The algorithm's three phases map cleanly onto the existing codebase — `g.Subgraph()` handles ego-net extraction, `CommunityDetector.Detect()` handles local and persona-graph detection, and the new `OverlappingCommunityDetector` interface mirrors the existing `CommunityDetector` pattern for a symmetric caller experience. The implementation should be sequential for v1.2 (goroutine parallelism deferred); the 200–300ms target for a 10K-node graph is achievable sequentially given that persona graphs are 2–3x the original size and Louvain at 10K runs in ~48ms.

The critical risks are implementation-correctness traps, not architectural unknowns. Nine specific pitfalls are fully documented (EGO-CRIT-01 through -09), covering the three most dangerous: (1) accidentally including the ego node in its own ego-net, which eliminates all persona splitting; (2) persona NodeID space collision with original NodeIDs; and (3) using standard (non-overlapping) NMI for accuracy validation, which silently validates the wrong property. All nine are preventable with explicit unit test assertions at each algorithm boundary.

## Key Findings

### Recommended Stack

The entire implementation lives inside the existing `package graph` with no new dependencies. Go 1.26.1 stdlib covers all requirements: `slices` and `maps` (1.21+) for deterministic sorted iteration and partition copying, `sync.Pool` for scratch buffer reuse (already the established pattern in `louvain_state.go` and `leiden_state.go`), and `math/rand` for seeded determinism. The persona graph is a standard `*Graph` with NodeIDs in a new numeric range — no new graph type is needed. The development toolchain (`go test -race`, `-bench`, `-benchmem`, `benchstat`, `pprof`) is unchanged from v1.0/v1.1.

**Core technologies:**
- `package graph` (internal): Graph, NodeID, CommunityDetector, Subgraph — all reused directly without modification
- `slices` / `maps` stdlib: deterministic iteration and partition copying — already used throughout codebase
- `sync.Pool`: scratch buffer reuse for inner Detect calls — established pattern in louvain_state.go
- `math/rand` (seeded): reproducibility for persona assignment — same pattern as existing detectors

### Expected Features

**Must have (table stakes):**
- `OverlappingCommunityDetector` interface with `Detect(*Graph) (OverlappingCommunityResult, error)` — symmetric with existing `CommunityDetector`
- `OverlappingCommunityResult` with `Communities [][]NodeID` and `NodeCommunities map[NodeID][]int` (forward + inverse index)
- Algorithm 1: ego-net extraction per node via `g.Subgraph(neighbors)` + local community detection
- Algorithm 2: persona graph construction with co-membership edge wiring and disjoint PersonaID namespace
- Algorithm 3: persona graph detection + overlapping community recovery (collect ALL persona memberships per original node)
- Pluggable inner detector: `EgoSplittingOptions.Inner CommunityDetector` accepts any `CommunityDetector`
- Edge case guards: empty graph, single node, directed graph (`ErrDirectedNotSupported`)
- Accuracy validation: Omega index (primary) + best-match F1 (secondary) on Karate Club, Football, Polbooks
- Performance benchmark: 10K-node synthetic graph target 200–300ms

**Should have (competitive):**
- Any inner detector choice (Louvain or Leiden) — unlike most overlapping libraries that hardcode the inner algorithm
- `OverlappingRatio float64` in result — fraction of nodes bridging communities; useful diagnostic for GraphRAG callers
- Concurrent ego-net detection (goroutine worker pool, bounded to `runtime.NumCPU()`) — defer until profiling confirms Algorithm 1 is bottleneck

**Defer (v2+):**
- Overlapping NMI (Lancichinetti 2009 / McDaid 2011 NMI_max) — non-trivial to implement; Omega index + F1 sufficient for v1.2
- Expose persona graph in `OverlappingCommunityResult` — trigger: explicit caller request for persona-level analysis
- Fuzzy/probabilistic membership — different algorithm family (BigCLAM, MNMF), out of scope
- Directed graph support — requires new ego-net definition; reject with error in v1.2
- Streaming / incremental persona graph updates — research-grade, significant API change

### Architecture Approach

The ego-splitting layer sits above the existing `CommunityDetector` implementations and satisfies a new `OverlappingCommunityDetector` interface. It calls the inner detector N+1 times per `Detect` invocation (N ego-nets + 1 persona graph) without modifying any existing code. The `egoSplittingDetector` struct is stateless — all mutable state is heap-allocated per `Detect` call — matching the existing Louvain/Leiden concurrency model. Four new files are added; zero existing files are modified.

**Major components:**
1. `overlapping_detector.go` — `OverlappingCommunityDetector` interface, `OverlappingCommunityResult`, `EgoSplittingOptions`, `NewEgoSplitting` constructor
2. `ego_splitting.go` — `egoSplittingDetector` struct, `Detect` method orchestrating Algorithms 1–3, `buildEgoNets` function
3. `persona.go` — `personaMap` type (unexported), `buildPersonaGraph`, `mapPersonasToOriginal`; internal only, scoped to one `Detect` call lifetime
4. `ego_splitting_test.go` — unit tests per algorithm step, integration tests on all 3 benchmark fixtures, Omega/F1 accuracy assertions, performance benchmark

### Critical Pitfalls

1. **EGO-CRIT-01: Ego node included in its own ego-net** — Pass only `neighbors` to `g.Subgraph()`, never append `v` itself. Warning sign: every ego-net yields a single community and persona count equals original node count.

2. **EGO-CRIT-02: Persona NodeID collision with original NodeIDs** — Use an independent monotonic counter for PersonaIDs, never reuse original NodeIDs. Assert `personaToOriginal` map is bijective and keys never overlap `[0, g.NodeCount())`.

3. **EGO-CRIT-03: Double-counted edges in persona graph** — Undirected graph iteration visits each edge twice; deduplicate with a canonical-key set before calling `personaGraph.AddEdge`. Assert `personaGraph.TotalWeight() == g.TotalWeight()`.

4. **EGO-CRIT-05: Incomplete community recovery in Algorithm 3** — Collect ALL community IDs across all of a node's personas, not just the first. Assert that at least one node has multiple memberships on Karate Club.

5. **EGO-CRIT-06: Wrong NMI variant for overlapping accuracy** — Standard `nmi()` is undefined for overlapping assignments and will produce misleadingly high scores when detection degrades to non-overlapping. Implement Omega index or `overlapNMI` (McDaid 2011 NMI_max) as the primary gate.

## Implications for Roadmap

Based on research, the architecture document's 6-phase build order maps directly onto a roadmap. Each phase leaves the package in a passing-tests state and gates the next phase.

### Phase 1: Types and Interfaces
**Rationale:** All subsequent code depends on the interface and result type definitions. Zero risk — purely additive, no logic yet. Establishes the API contract early so tests can be written before implementation is complete.
**Delivers:** `OverlappingCommunityDetector` interface, `OverlappingCommunityResult` struct, `EgoSplittingOptions`, `NewEgoSplitting` stub (returns unimplemented error). Package compiles; all existing tests pass.
**Addresses:** Table-stakes interface symmetry with `CommunityDetector`; directed graph guard (EGO-CRIT-09).
**Avoids:** Anti-pattern of merging into existing `CommunityDetector` interface, which would be a breaking change.

### Phase 2: Persona Graph Infrastructure
**Rationale:** The persona mapping layer (`persona.go`) contains two independent critical pitfalls (EGO-CRIT-02, EGO-CRIT-03). Validating it in isolation on hand-crafted small graphs makes bugs cheap to catch before they compound with Algorithm 1 bugs.
**Delivers:** `personaMap` type, `buildPersonaGraph`, `mapPersonasToOriginal`. Unit tests asserting: (a) persona IDs never overlap original IDs, (b) `personaGraph.TotalWeight() == g.TotalWeight()`, (c) inverse map is bijective.
**Addresses:** EGO-CRIT-02 (NodeID collision), EGO-CRIT-03 (double-counted edges).
**Avoids:** Silent edge-weight doubling and NodeID aliasing before they are entangled with Algorithm 1 logic.

### Phase 3: Algorithm 1 — Ego-Net Construction
**Rationale:** `buildEgoNets` is independent of `buildPersonaGraph` at the code level. Testing ego-net extraction in isolation surfaces EGO-CRIT-01 (ego node inclusion) immediately — the most common first-pass correctness error.
**Delivers:** `buildEgoNets(g, inner, minSize)` function. Unit tests asserting: ego-net for node `v` contains no entry equal to `v`; inner `Detect` called once per node; degenerate cases (degree 0, degree 1) return correctly.
**Addresses:** EGO-CRIT-01 (ego node in ego-net), EGO-CRIT-04 (star-center persona explosion — surface `MinEgoNetSize` option).
**Avoids:** Including ego node in subgraph; unbounded persona count on star-topology inputs.

### Phase 4: Full Detect Pipeline (Algorithms 2 + 3) and Integration Tests
**Rationale:** Wire the three phases together into `egoSplittingDetector.Detect`. Integration tests on Karate Club, Football, and Polbooks confirm end-to-end correctness and surface EGO-CRIT-05 (wrong community recovery).
**Delivers:** Complete `Detect` method. Integration tests: result is non-empty; at least one node has `len(communities) > 1` on Karate Club; `go test -race` passes (sequential implementation, no goroutines to race).
**Addresses:** EGO-CRIT-05 (incomplete community recovery).
**Avoids:** Taking only the first persona's community assignment; silently degrading to non-overlapping behavior.

### Phase 5: Accuracy Validation and Performance Benchmarks
**Rationale:** Accuracy tests require a working end-to-end pipeline from Phase 4. Omega index and best-match F1 are the correct metrics — not standard NMI. Performance benchmarks on 10K-node graphs confirm the 200–300ms target.
**Delivers:** `OmegaIndex`, `BestMatchF1` functions. Accuracy tests with empirically-calibrated thresholds on all three fixture graphs. `BenchmarkEgoSplitting` at 10K nodes.
**Addresses:** EGO-CRIT-06 (wrong NMI variant), 10K performance target, quality gate for v1.2.
**Avoids:** Using `nmi()` for overlapping validation; treating non-overlapping degradation as a passing test.

### Phase 6: Edge Cases and Hardening
**Rationale:** Edge cases are cheap to add after the main pipeline is validated. They protect against panics in production callers that pass degenerate inputs.
**Delivers:** Guards and tests for empty graph, single-node graph, disconnected graph, star graph, all-isolated nodes, directed graph. All "Looks Done But Isn't" checklist items from PITFALLS.md verified as test assertions.
**Addresses:** All EGO-CRIT-0X warning-sign assertions; robust directed-graph rejection.
**Avoids:** Panics on degenerate inputs; silent incorrect results without error return.

### Phase Ordering Rationale

- **Types before logic (Phase 1 first):** Prevents interface churn mid-implementation. The `OverlappingCommunityResult` shape propagates through all downstream phases.
- **Persona infrastructure before algorithms (Phase 2 before 3):** Persona ID correctness is hard to debug once entangled with ego-net logic; independent validation is essential.
- **Sequential before parallel:** EGO-CRIT-07 and EGO-CRIT-08 (pool thrashing, race conditions) apply only to parallel ego-net detection. The 200–300ms target is achievable sequentially. Parallelism is a post-v1.2 optimization.
- **Accuracy metrics after full pipeline (Phase 5 after 4):** Omega index requires a working `Detect` call; implementing metrics against a stub wastes time.
- **Edge cases last (Phase 6):** Guards on broken logic add no value. Edge-case paths require a stable happy path.

### Research Flags

Phases with standard patterns (skip deeper research):
- **Phase 1:** Interface definition follows the established `CommunityDetector` pattern exactly.
- **Phase 3:** `g.Subgraph()` already exists; `inner.Detect()` already works. Pure reuse.
- **Phase 4:** Algorithm 3 backprojection is a straightforward map inversion. Well-documented in paper.
- **Phase 6:** Guard patterns already established by `ErrDirectedNotSupported` in Louvain/Leiden.

Phases that may benefit from deeper research during planning:
- **Phase 2:** The co-membership edge-wiring condition in Algorithm 2 is subtle: edge (u,v) goes to persona pair (pu, pv) only when u and v co-appear in the same local community in BOTH u's and v's ego-nets. The `commInEgoNet[u][v]` lookup structure needs careful design; validate against paper Section 2.2 before implementing.
- **Phase 5:** Omega index (O(n^2) pairwise) needs threshold calibration per fixture. Best-match F1 matching strategy (greedy vs optimal) may affect scores. Plan time to determine empirical thresholds after Phase 4 produces results.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Zero new deps; all patterns confirmed in existing codebase. No unknowns. |
| Features | HIGH | Paper is precise (KDD 2017, widely cited). Algorithm steps are unambiguous. MVP scope is conservative and well-bounded. |
| Architecture | HIGH | Existing codebase patterns are directly reusable. Interface design follows established precedent. Persona graph data structure decision confirmed correct. |
| Pitfalls | HIGH | 9 critical pitfalls identified with concrete prevention code and warning signs. All are detectable by unit assertions; none require architectural redesign to fix. |

**Overall confidence:** HIGH

### Gaps to Address

- **Omega index threshold calibration:** Correct threshold values for Karate Club, Football, and Polbooks accuracy gates are not yet known. Determine empirically once Phase 4 delivers working results, then harden as test constants. Do not set speculatively.
- **Algorithm 2 edge-wiring performance at high degree:** The `commInEgoNet[u][v]` lookup table is O(N * avg_degree) in memory — acceptable at avg_degree 20 and N 10K (~200K entries). For avg_degree > 100, this becomes a memory concern. Flag for profiling in Phase 5 benchmarks.
- **Persona graph size bounds on adversarial inputs:** The paper reports 2–3x node expansion on social graphs. Adversarial inputs can push expansion toward O(|E|). The `MinEgoNetSize` option partially mitigates this; validate empirically during Phase 3 on a star-topology benchmark.
- **Overlapping NMI (McDaid 2011) deferred:** If accuracy reporting requirements increase post-v1.2, `overlapNMI` (NMI_max formulation) must be implemented before any public API claims NMI as a quality metric.

## Sources

### Primary (HIGH confidence)
- Epasto, A., Lattanzi, S., Paes Leme, R. — "Ego-Splitting Framework: from Non-Overlapping to Overlapping Clusters." KDD 2017. arxiv:1707.04692 — Algorithms 1, 2, 3; persona graph definition; ego-net (egoless) definition
- Existing `package graph` codebase — `graph.go`, `detector.go`, `louvain.go`, `leiden.go`, `louvain_state.go`, `leiden_state.go` — confirmed API contracts and reuse points
- Go 1.26.1 stdlib — `slices`, `maps`, `sync`, `math/rand` — all available in go.mod; zero new imports needed

### Secondary (MEDIUM confidence)
- Collins, L.M., Dent, C.W. (1988). "Omega: A general formulation of the Rand Index." Multivariate Behavioral Research 23.2 — Omega index original definition
- Gregory, S. (2011). "Fuzzy overlapping communities in networks." Journal of Statistical Mechanics — Omega index adapted for overlapping communities
- Lancichinetti, A., Fortunato, S. (2009). "Benchmarks for testing community detection algorithms." Physical Review E 80 — LFR overlapping benchmark; overlapping NMI

### Tertiary (LOW confidence)
- McDaid, A.F., Greene, D., Hurley, N. (2011). "Normalized Mutual Information to Evaluate Overlapping Community Finding Algorithms." arXiv:1110.2515 — NMI_max formulation; deferred to post-v1.2; not yet validated against an implementation

---
*Research completed: 2026-03-30*
*Ready for roadmap: yes*
