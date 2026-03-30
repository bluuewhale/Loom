# Milestones

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
