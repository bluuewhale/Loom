# Codebase Structure

**Analysis Date:** 2026-03-29

## Directory Layout

```
community-detection/         # Go module root (module: community-detection)
├── go.mod                   # Module declaration, no external dependencies
├── CLAUDE.md                # Project-level agent instructions
├── .gitignore               # Ignores .planning/
├── .planning/               # GSD workflow artifacts (not committed)
│   ├── codebase/            # Codebase analysis docs (STACK.md, ARCHITECTURE.md, etc.)
│   └── phases/              # Per-phase plan and verification docs
└── graph/                   # Single Go package — all graph primitives and algorithms
    ├── graph.go             # Core Graph type, NodeID, Edge, all graph methods
    ├── graph_test.go        # Unit tests for graph operations
    ├── modularity.go        # ComputeModularity / ComputeModularityWeighted
    ├── modularity_test.go   # Unit + integration tests for modularity Q
    ├── registry.go          # NodeRegistry (string↔NodeID mapping)
    ├── registry_test.go     # Unit tests for NodeRegistry
    └── testdata/
        └── karate.go        # Zachary Karate Club fixture (edges + partition)
```

## Directory Purposes

**`graph/`:**
- Purpose: The entire library lives here as `package graph`
- Contains: Core data structures, graph algorithms, node registry utility
- Key files: `graph/graph.go`, `graph/modularity.go`, `graph/registry.go`

**`graph/testdata/`:**
- Purpose: Canonical fixture data for integration-level algorithm tests
- Contains: Go files in `package testdata` exposing named variable datasets
- Key files: `graph/testdata/karate.go`

**`.planning/`:**
- Purpose: GSD workflow artifacts — plans, phase specs, codebase analysis
- Generated: Manually by GSD skill invocations
- Committed: No (`.gitignore` excludes it)

## Key File Locations

**Entry Points:**
- `graph/graph.go`: `NewGraph(directed bool) *Graph` — construct a graph
- `graph/registry.go`: `NewRegistry() *NodeRegistry` — construct a name registry

**Core Logic:**
- `graph/graph.go`: All graph mutation (`AddNode`, `AddEdge`) and query (`Neighbors`, `Strength`, `TotalWeight`, `Clone`, `Subgraph`, `WeightToComm`, `CommStrength`)
- `graph/modularity.go`: Newman-Girvan modularity Q computation

**Test Fixtures:**
- `graph/testdata/karate.go`: `KarateClubEdges` and `KarateClubPartition`

**Configuration:**
- `go.mod`: Module name `community-detection`, Go version `1.26.1`, zero external dependencies

## Naming Conventions

**Files:**
- Snake-case, noun-based: `graph.go`, `modularity.go`, `registry.go`
- Test files: `<source>_test.go` co-located with source (e.g., `graph_test.go`)
- Fixture files: noun describing dataset (`karate.go`)

**Directories:**
- Lowercase single-word: `graph/`, `testdata/`

**Types:**
- PascalCase exported types: `Graph`, `NodeID`, `Edge`, `NodeRegistry`

**Functions / Methods:**
- PascalCase exported: `NewGraph`, `AddEdge`, `ComputeModularity`, `Register`
- camelCase unexported: `directed`, `nodes`, `adjacency`, `totalWeight` (struct fields)

**Variables / Constants:**
- Exported fixture vars: PascalCase descriptive (`KarateClubEdges`, `KarateClubPartition`)

## Where to Add New Code

**New graph algorithm (e.g., Louvain, Girvan-Newman):**
- Implementation: `graph/<algorithm-name>.go` (e.g., `graph/louvain.go`)
- Tests: `graph/<algorithm-name>_test.go` (e.g., `graph/louvain_test.go`)
- Use `KarateClubEdges` + `KarateClubPartition` for integration assertions

**New graph operation / method:**
- Add to `graph/graph.go` as a method on `*Graph`
- Add corresponding test cases to `graph/graph_test.go`

**New test fixture dataset:**
- Add as a new file in `graph/testdata/` with `package testdata`
- Export as named `var` arrays or maps following `KarateClub*` naming pattern

**New utility (non-graph-algorithm):**
- Create `graph/<utility-name>.go` with `package graph`
- Co-locate tests in `graph/<utility-name>_test.go`

**New top-level package (future, e.g., CLI, HTTP API):**
- Create a new directory at module root: `cmd/`, `api/`, etc.
- Import `community-detection/graph` as a library dependency

## Special Directories

**`graph/testdata/`:**
- Purpose: Benchmark and integration fixture data
- Generated: No — hand-authored from published datasets
- Committed: Yes

**`.planning/`:**
- Purpose: GSD workflow: codebase analysis, phase plans, verification checklists
- Generated: Yes — by GSD skill invocations
- Committed: No (`.gitignore`)

---

*Structure analysis: 2026-03-29*
