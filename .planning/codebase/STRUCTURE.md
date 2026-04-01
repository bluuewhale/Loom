# Codebase Structure

**Analysis Date:** 2026-04-01

## Directory Layout

```
loom/                           # Module root: github.com/bluuewhale/loom
├── graph/                      # Entire production library (package graph)
│   ├── graph.go                # Graph type, NodeID, Edge, all graph methods
│   ├── registry.go             # NodeRegistry: string↔NodeID bidirectional mapping
│   ├── detector.go             # CommunityDetector interface, options types, constructors
│   ├── louvain.go              # Louvain algorithm: Detect, phase1, buildSupergraph
│   ├── louvain_state.go        # louvainState struct, sync.Pool, reset logic
│   ├── leiden.go               # Leiden algorithm: Detect, runOnce, refinePartition
│   ├── leiden_state.go         # leidenState struct, sync.Pool, reset logic
│   ├── modularity.go           # ComputeModularity, ComputeModularityWeighted
│   ├── omega.go                # OmegaIndex overlapping community quality metric
│   ├── ego_splitting.go        # Ego-splitting: Detect, Update, buildPersonaGraph, incremental
│   ├── testdata/               # Embedded benchmark graph datasets
│   │   ├── karate.go           # Zachary's Karate Club graph (34 nodes)
│   │   ├── football.go         # American college football network
│   │   └── polbooks.go         # Political books network
│   ├── graph_test.go           # Unit tests for Graph methods
│   ├── registry_test.go        # Unit tests for NodeRegistry
│   ├── detector_test.go        # Unit tests for detector constructors/options
│   ├── louvain_test.go         # Unit + integration tests for Louvain
│   ├── leiden_test.go          # Unit + integration tests for Leiden
│   ├── leiden_numruns_test.go  # Multi-run Leiden behavior tests
│   ├── modularity_test.go      # Unit tests for modularity computation
│   ├── omega_test.go           # Unit tests for OmegaIndex
│   ├── ego_splitting_test.go   # Unit + integration tests for ego-splitting
│   ├── accuracy_test.go        # Cross-algorithm accuracy tests on real datasets
│   ├── benchmark_test.go       # Performance benchmarks (Louvain/Leiden/EgoSplitting)
│   ├── example_test.go         # Runnable godoc examples
│   ├── testhelpers_test.go     # Shared test utilities (floatEq, etc.)
│   ├── norace_test.go          # Tests excluded under -race flag
│   └── race_test.go            # Tests requiring -race detection
├── scripts/                    # Build/tooling scripts
├── .planning/                  # GSD planning artifacts (committed to git)
│   ├── codebase/               # Codebase analysis documents (this file's home)
│   ├── milestones/             # Per-milestone planning and audit files
│   ├── phases/                 # Per-phase execution plans
│   ├── STATE.md                # Current project state (milestone, phase, progress)
│   ├── ROADMAP.md              # High-level milestone roadmap
│   └── REQUIREMENTS.md         # Living requirements document
├── .github/                    # CI configuration
├── go.mod                      # Module: github.com/bluuewhale/loom, go 1.26, no requires
├── bench-baseline.txt          # Committed benchmark baseline (Apple M4, arm64)
└── README.md                   # Public-facing documentation
```

## Directory Purposes

**`graph/` (production):**
- Purpose: The entire library — all algorithm implementations, interfaces, and utilities
- Contains: 10 production `.go` files, 1 sub-package (`testdata/`), 14 test files
- Key files: `graph.go` (foundation), `detector.go` (public interface), `ego_splitting.go` (largest file, ~900 lines)
- Note: Single flat package; no internal sub-packages. All identifiers are package-level visible within tests.

**`graph/testdata/`:**
- Purpose: Canonical benchmark graphs embedded as Go source (not files)
- Contains: `karate.go`, `football.go`, `polbooks.go` — each exposes a constructor returning `*graph.Graph`
- Generated: No — hand-encoded from published datasets
- Committed: Yes — part of test infrastructure

**`.planning/`:**
- Purpose: GSD workflow planning artifacts — treated as project history, not generated output
- Contains: Milestone roadmaps, phase execution plans, codebase analysis, project state
- Generated: No — authored by GSD workflow
- Committed: Yes

**`scripts/`:**
- Purpose: Build and tooling helpers
- Contains: Shell scripts for CI or local dev tasks

## Key File Locations

**Entry Points (Public API):**
- `graph/detector.go`: `NewLouvain`, `NewLeiden`, `CommunityDetector`, `CommunityResult`
- `graph/ego_splitting.go`: `NewEgoSplitting`, `NewOnlineEgoSplitting`, `OverlappingCommunityDetector`, `OnlineOverlappingCommunityDetector`, `GraphDelta`
- `graph/graph.go`: `NewGraph`, `Graph`, `NodeID`, `Edge`
- `graph/registry.go`: `NewRegistry`, `NodeRegistry`
- `graph/modularity.go`: `ComputeModularity`, `ComputeModularityWeighted`
- `graph/omega.go`: `OmegaIndex`

**Algorithm Implementations:**
- `graph/louvain.go`: `phase1`, `buildSupergraph`, `normalizePartition`, `reconstructPartition`, `deltaQ` (dead code)
- `graph/leiden.go`: `refinePartition`, `runOnce`
- `graph/ego_splitting.go`: `buildPersonaGraph`, `buildPersonaGraphIncremental`, `buildEgoNet`, `computeAffected`, `runParallelEgoNets`, `warmStartedDetector`, `cloneDetector`

**State / Pool Management:**
- `graph/louvain_state.go`: `louvainState`, `louvainStatePool`, `acquireLouvainState`, `releaseLouvainState`, `newLouvainState` (unused)
- `graph/leiden_state.go`: `leidenState`, `leidenStatePool`, `acquireLeidenState`, `releaseLeidenState`, `newLeidenState` (unused)

**Testing Infrastructure:**
- `graph/testhelpers_test.go`: `floatEq` and other shared test helpers
- `graph/testdata/karate.go`: Zachary Karate Club — primary small-graph accuracy fixture
- `graph/accuracy_test.go`: NMI/Omega accuracy assertions across real datasets
- `graph/benchmark_test.go`: `BenchmarkLouvain1K/10K`, `BenchmarkLeiden1K/10K`, warm-start benchmarks

**Configuration:**
- `go.mod`: Module path and Go version (1.26), zero external dependencies
- `bench-baseline.txt`: Reference benchmark output on Apple M4 arm64

## Naming Conventions

**Files:**
- Algorithm implementations: `<algorithm>.go` (e.g., `louvain.go`, `leiden.go`)
- State/pool for algorithm: `<algorithm>_state.go` (e.g., `louvain_state.go`)
- Tests: `<subject>_test.go` (co-located with production code, same directory)
- Benchmarks: included in `benchmark_test.go` (not per-algorithm)
- Accuracy tests: `accuracy_test.go` (cross-algorithm correctness, separate from unit tests)

**Types:**
- Exported types: `PascalCase` (e.g., `NodeID`, `Graph`, `CommunityResult`, `GraphDelta`)
- Unexported concrete detector types: `<algorithm>Detector` (e.g., `louvainDetector`, `leidenDetector`, `egoSplittingDetector`)
- State structs: `<algorithm>State` (e.g., `louvainState`, `leidenState`)

**Functions:**
- Public constructors: `New<Type>` (e.g., `NewGraph`, `NewLouvain`, `NewEgoSplitting`)
- Pool acquire/release: `acquire<Type>State` / `release<Type>State`
- Internal phases: lowercase verb phrases (`phase1`, `buildSupergraph`, `refinePartition`, `buildPersonaGraph`)

**Test functions:**
- Unit tests: `Test<Subject><Behavior>` (e.g., `TestWeightToComm`, `TestLouvainKarate`)
- Benchmarks: `Benchmark<Algorithm><Scale>` (e.g., `BenchmarkLouvain1K`, `BenchmarkLeiden10K`)
- Examples: `Example<Function>` (godoc convention)

## Where to Add New Code

**New disjoint community detection algorithm (e.g., Infomap):**
- Interface declaration: add constructor to `graph/detector.go` (options struct + `NewX` function)
- Implementation: new file `graph/infomap.go`
- State (if needed): new file `graph/infomap_state.go`
- Tests: new file `graph/infomap_test.go`
- Add to `warmStartedDetector` and `cloneDetector` switch in `graph/ego_splitting.go`

**New overlapping community detection algorithm:**
- Implement `OverlappingCommunityDetector` or `OnlineOverlappingCommunityDetector` from `graph/ego_splitting.go`
- New file: `graph/<algorithm>.go`
- Tests: `graph/<algorithm>_test.go`

**New graph utility method (e.g., degree, path length):**
- Add to `graph/graph.go` as a method on `*Graph`
- Tests: `graph/graph_test.go`

**New quality metric (e.g., NMI, conductance):**
- New file: `graph/<metric>.go`
- Tests: `graph/<metric>_test.go`

**New benchmark:**
- Add to `graph/benchmark_test.go` alongside existing benchmarks
- Update `bench-baseline.txt` after running on reference hardware

**New real-world test dataset:**
- Add to `graph/testdata/` as a new `.go` file following the pattern in `karate.go`
- Reference from `accuracy_test.go`

## Special Directories

**`graph/testdata/`:**
- Purpose: Canonical graph datasets encoded as Go source for use in tests and benchmarks
- Generated: No
- Committed: Yes
- Note: Lives inside `graph/` so it is within `package graph`'s test scope; accessed as `testdata.Karate()` etc.

**`.planning/`:**
- Purpose: GSD workflow state — roadmaps, phase plans, codebase analysis
- Generated: No (authored)
- Committed: Yes — planning files are project history per CLAUDE.md

**`.github/`:**
- Purpose: GitHub Actions CI configuration
- Generated: No
- Committed: Yes

---

*Structure analysis: 2026-04-01*
