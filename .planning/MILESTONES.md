# Milestones

## graph-core-opt Graph Core & Leiden Performance (Shipped: 2026-04-01)

**Phases completed:** 3 phases, 6 plans
**Branch:** feat/graph-core-optimization
**Timeline:** 2026-04-01

**Key accomplishments:**

- Nodes() sorted-slice cache + math/rand/v2 PCG zero-alloc reseed + dead code removal (deltaQ, newLouvainState, newLeidenState)
- Zero-copy csrGraph adjacency view + index-shuffle in phase1 hot loop — BFS cursor fix eliminates queue reslicing
- sync.Pool for Subgraph seen-map — eliminates per-ego-net map allocation across all EgoSplitting calls
- refinePartitionInPlace: CSR-indexed bool scratch + sorted commNodePairs — Leiden 10K 58,220 → 45,871 allocs/op (−21.3%)
- Counting sort (O(N) with sparse reset) + int32 CSR BFS queue — Leiden 10K 60.4ms → 59.1ms (−2.2% ns/op)
- Louvain 10K: 63.5ms → 55.1ms (−13.2% ns/op), 48,773 → 45,909 allocs/op (−5.9%)

---

## v1.3 Online Ego-Splitting (Shipped: 2026-03-31)

**Phases completed:** 4 phases, 6 plans — graph/ego_splitting.go: 942 LOC | tests: 1713 LOC

**Key accomplishments:**

- `OnlineOverlappingCommunityDetector` interface + `NewOnlineEgoSplitting` constructor with stable `Update(g, delta, prior)` API, empty-delta 0-alloc fast-path, and directed guard
- Incremental pipeline: `computeAffected` scopes ego-net rebuilds to affected nodes only; `buildPersonaGraphIncremental` carries over unaffected PersonaIDs; `warmStartedDetector` warm-starts global Louvain/Leiden from prior partition
- Parallel ego-net goroutine pool (GOMAXPROCS workers) reduces `BenchmarkEgoSplitting10K` from ~1500ms to 233ms/op (target ≤300ms)
- `BenchmarkUpdate1Node` achieves 29x speedup over full `Detect()` on Karate Club + 1 node (ONLINE-08 ≥10x target)
- `assertResultInvariants` helper + 6-case invariant test suite + `TestEgoSplittingConcurrentUpdate` — zero race reports under `go test -race`

---

## v1.2 Overlapping Community Detection (Shipped: 2026-03-31)

**Phases completed:** 4 phases, 6 plans, 2 tasks

**Key accomplishments:**

- OverlappingCommunityDetector interface and EgoSplitting stub declared in graph/ego_splitting.go with full nil-defaulting constructor and ErrNotImplemented sentinel
- One-liner:
- One-liner:
- Task 1 — OmegaIndex (`graph/omega.go`)
- Task 1 — Accuracy + Race Tests (`graph/ego_splitting_test.go`)
- Task 1 — `graph/ego_splitting.go`:

---

## v1.1 Online Community Detection (Shipped: 2026-03-30)

**Phases completed:** 1 phase, 2 plans, 4 tasks

**Key accomplishments:**

- Added `InitialPartition map[NodeID]int` to `LouvainOptions` and `LeidenOptions` — nil = cold start, zero breaking change to v1.0 API
- Warm-seed `reset()` logic in both state files: maxCommID offset for new nodes, 0-indexed ID compaction, commStr rebuilt from current graph strengths
- `firstPass` guard in both `Detect()` loops ensures warm partition applied only on original graph; supergraph passes always cold-reset
- 4 warm-start correctness tests across 3 fixtures (Karate Club, Football, Polbooks) verifying Q(warm) ≥ Q(cold_perturbed)
- `BenchmarkLouvainWarmStart` and `BenchmarkLeidenWarmStart` with correct measurement structure (setup outside ResetTimer)

---

## v1.0 Community Detection (Shipped: 2026-03-29)

**Phases completed:** 4 phases, 5 plans, 7 tasks

**Key accomplishments:**

- `CommunityDetector` interface with swappable `NewLouvain`/`NewLeiden` constructors, `CommunityResult`/options types
- Complete Louvain community detection: phase1 deltaQ local moves, buildSupergraph compression, convergence loop — Karate Club Q=0.4156
- Leiden algorithm with BFS refinement for connected-community guarantee — NMI=0.716 on Karate Club
- Football (115-node/613-edge) and Polbooks (105-node/441-edge) fixtures; NMI accuracy suite validates both algorithms on 3 benchmarks
- 10K-node benchmarks: Louvain ~48ms/op, Leiden ~57ms/op — both under 100ms target; race-free with `sync.Pool`

---
