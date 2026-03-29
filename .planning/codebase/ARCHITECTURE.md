# Architecture

**Analysis Date:** 2026-03-29

## Pattern Overview

**Overall:** Single-package library — all graph primitives, algorithms, and helpers live under one Go package (`graph`).

**Key Characteristics:**
- No layering between domain types; everything in `package graph`
- Algorithms are free functions operating on `*Graph` and `map[NodeID]int` (partition)
- Name-to-ID translation is a separate optional utility (`NodeRegistry`) — callers can use integer IDs directly
- No external dependencies; pure standard-library Go module

## Layers

**Core Data Structure:**
- Purpose: Represent weighted directed/undirected graphs in memory
- Location: `graph/graph.go`
- Contains: `NodeID`, `Edge`, `Graph` struct, all graph mutation and query methods
- Depends on: nothing
- Used by: algorithms (`modularity.go`), tests, external callers

**Algorithm Layer:**
- Purpose: Graph-theoretic computations (currently: Newman-Girvan modularity Q)
- Location: `graph/modularity.go`
- Contains: `ComputeModularity`, `ComputeModularityWeighted`
- Depends on: `*Graph` public API (`Nodes`, `Neighbors`, `Strength`, `TotalWeight`)
- Used by: community-detection algorithm callers (future phases)

**Utility / Bridge Layer:**
- Purpose: Map human-readable string node names to integer `NodeID`s
- Location: `graph/registry.go`
- Contains: `NodeRegistry` struct, `Register`, `ID`, `Name`, `Len`
- Depends on: `NodeID` type from `graph/graph.go`
- Used by: callers that load graphs from string-labeled datasets

**Test Fixtures:**
- Purpose: Canonical benchmark graphs for algorithm validation
- Location: `graph/testdata/karate.go`
- Contains: `KarateClubEdges` (78 undirected edges, nodes 0–33), `KarateClubPartition` (ground-truth two-community split, Q ≈ 0.371)
- Depends on: nothing (plain data; `package testdata`)
- Used by: `graph/modularity_test.go`

## Data Flow

**Building a graph from string data:**

1. Caller creates `NodeRegistry` via `NewRegistry()`
2. For each named edge, call `registry.Register(name)` → returns `NodeID`
3. Call `graph.AddEdge(fromID, toID, weight)` with those `NodeID`s
4. Graph is ready for algorithmic use

**Computing modularity:**

1. Provide `*Graph` and a `map[NodeID]int` partition (node → community label)
2. Call `ComputeModularity(g, partition)` or `ComputeModularityWeighted(g, partition, resolution)`
3. Function iterates all nodes via `g.Nodes()`, accumulates per-community `intraWeight` and `degSum`
4. Returns scalar `float64` Q value

**State Management:**
- All state is held in `*Graph` (adjacency list + node weights + totalWeight flag)
- `NodeRegistry` is independent state; no cross-reference to `*Graph`
- No global mutable state; all types are value-safe when used by a single goroutine

## Key Abstractions

**NodeID:**
- Purpose: Type-safe integer identifier for graph nodes; prevents mixing raw `int` values
- Examples: `graph/graph.go` line 4
- Pattern: `type NodeID int` — typed alias

**Graph:**
- Purpose: Adjacency-list weighted graph supporting both directed and undirected modes
- Examples: `graph/graph.go`
- Pattern: Pointer receiver methods; constructed via `NewGraph(directed bool)`; auto-creates nodes on `AddEdge`

**Partition (`map[NodeID]int`):**
- Purpose: Encodes community assignment — maps each node to a community label integer
- Examples: `graph/modularity.go`, `graph/testdata/karate.go`
- Pattern: Plain Go map; nodes absent from partition default to community `-1` (singleton)

**NodeRegistry:**
- Purpose: Bidirectional name↔ID mapping; enables string-labeled graph construction
- Examples: `graph/registry.go`
- Pattern: Forward lookup via `map[string]NodeID`, reverse via `[]string` indexed by `NodeID`

## Entry Points

**Library consumers start here:**
- Location: `graph/graph.go` — `NewGraph(directed bool) *Graph`
- Triggers: Any code importing `community-detection/graph`
- Responsibilities: Construct the primary graph object

**Algorithm entry point:**
- Location: `graph/modularity.go` — `ComputeModularity` / `ComputeModularityWeighted`
- Triggers: Called after graph construction and partition assignment
- Responsibilities: Return scalar modularity score Q

**Test entry points:**
- Location: `graph/graph_test.go`, `graph/modularity_test.go`, `graph/registry_test.go`
- Triggers: `go test ./graph/...`
- Responsibilities: Verify graph operations, modularity formula correctness, registry CRUD

## Error Handling

**Strategy:** No explicit error returns — callers are expected to use valid inputs.

**Patterns:**
- Out-of-range or unknown `NodeID` in `NodeRegistry.Name` returns `("", false)` — two-value idiom
- `NodeRegistry.ID` returns `(0, false)` for unregistered names — two-value idiom
- `Graph` auto-creates nodes on `AddEdge` rather than returning errors
- `ComputeModularity` returns `0.0` for edgeless graphs or empty partitions (documented guard)

## Cross-Cutting Concerns

**Logging:** None — library package; no logging framework used.
**Validation:** Inline guards in methods (existence checks, range checks); no separate validation layer.
**Authentication:** Not applicable (pure library, no I/O).
**Concurrency:** Not safe for concurrent use; documented on `NodeRegistry`. `Graph` has no concurrent-use guarantee either.

---

*Architecture analysis: 2026-03-29*
