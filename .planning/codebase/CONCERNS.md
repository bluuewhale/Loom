# Codebase Concerns

**Analysis Date:** 2026-03-29

## Tech Debt

**No module-level entry point or public API surface:**
- Issue: The module `community-detection` (`go.mod`) has no `main` package, no `cmd/` directory, and no exported top-level API beyond the `graph` package. There is no library entrypoint or CLI. Callers have no clear way to consume this as a library or tool.
- Files: `go.mod`, `graph/graph.go`, `graph/modularity.go`, `graph/registry.go`
- Impact: Cannot be imported as a Go module by external consumers; no CLI runner exists for end-to-end use.
- Fix approach: Add a `cmd/community-detection/main.go` CLI entry point, or define a public top-level `api.go` package that re-exports the graph primitives under a stable surface.

**`go.mod` specifies a non-existent Go version (`go 1.26.1`):**
- Issue: Go 1.26.1 does not exist as of 2026-03-29 (latest stable is 1.23.x). The `go` directive in `go.mod` is set to a future/invalid version. This may cause `go build` to fail or produce unexpected behavior on real toolchains.
- Files: `go.mod`
- Impact: Build failures on standard Go installations; CI may reject the module.
- Fix approach: Change `go 1.26.1` to the actual installed toolchain version (e.g., `go 1.23.0`).

**`Graph` struct fields are all unexported with no serialization support:**
- Issue: `Graph`, `NodeRegistry`, and all internal fields are package-private. There is no JSON/binary marshaling, no persistence, and no way to serialize or deserialize graph state across process boundaries.
- Files: `graph/graph.go`, `graph/registry.go`
- Impact: Any caller needing to persist or transmit graph data must reconstruct from scratch. Blocks real-world use cases (checkpointing, distributed algorithms).
- Fix approach: Implement `encoding.json.Marshaler`/`Unmarshaler` or a simple edge-list serialization format.

**Duplicate test coverage between table-driven and explicit tests:**
- Issue: `modularity_test.go` contains both a table-driven `TestModularityEdgeCases` (lines 160–263) and explicit individual tests (`TestModularityCompleteGraph`, `TestModularityRingGraph`, `TestModularityDisconnectedGraph`, lines 265–316) that test the exact same scenarios. This is redundant and increases maintenance burden.
- Files: `graph/modularity_test.go`
- Impact: Any change to test expectations must be made in two places; test suite is inflated without additional coverage.
- Fix approach: Remove the standalone explicit tests (lines 265–316) and rely solely on the table-driven suite, or vice versa.

**`NodeRegistry` not integrated with `Graph`:**
- Issue: `NodeRegistry` (`graph/registry.go`) is a standalone utility. `Graph` operates purely on `NodeID` integers. There is no constructor or factory that ties a `NodeRegistry` to a `Graph`, meaning callers must manually keep them in sync.
- Files: `graph/registry.go`, `graph/graph.go`
- Impact: Error-prone for callers; string-to-ID mapping can drift out of sync with the graph's node set. Auto-created nodes (via `AddEdge`) are invisible to the registry.
- Fix approach: Introduce a `NamedGraph` or `GraphBuilder` that wraps both, or add a `RegisteredGraph` type that delegates `AddEdge(string, string, float64)` through the registry.

**`AddEdge` silently auto-creates nodes with weight `1.0`:**
- Issue: When `AddEdge` is called with node IDs not previously registered via `AddNode`, nodes are created with a default weight of `1.0` without any notification to the caller (`graph/graph.go` lines 49–60). Node weights set via `AddNode` are intentional; silently-created nodes may have the wrong semantic weight.
- Files: `graph/graph.go`
- Impact: Subtle correctness bugs in weighted algorithms if callers expect all nodes to have custom weights.
- Fix approach: Document the behavior explicitly in the function signature, or add an `AddEdgeStrict` variant that panics/errors if nodes are missing.

## Known Bugs

**`EdgeCount` is incorrect for directed graphs with self-loops:**
- Symptoms: `EdgeCount` for directed graphs divides total adjacency entries by 2 only for undirected graphs (`graph/graph.go` lines 94–103). However, for undirected graphs with self-loops, `AddEdge` stores a self-loop only once in adjacency (line 64 guard `from != to`), so the divide-by-2 can produce a fractional count if the total is odd.
- Files: `graph/graph.go` lines 94–103
- Trigger: Undirected graph with an odd number of self-loops.
- Workaround: None; `EdgeCount` returns `total / 2` using integer division which silently floors.

**`Neighbors` returns `nil` for nodes that exist but have no edges, and also for nodes that do not exist at all:**
- Symptoms: `g.Neighbors(id)` returns `nil` whether `id` is a valid isolated node or a completely unknown node ID. Callers cannot distinguish "node exists with no edges" from "node does not exist."
- Files: `graph/graph.go` line 74–76
- Trigger: Call `Neighbors` on a non-existent node vs. an isolated node.
- Workaround: Check `NodeCount` or use `Nodes()` to verify existence first.

## Performance Bottlenecks

**`CommStrength` iterates entire partition on every call:**
- Problem: `CommStrength` iterates over every entry in the partition map to find nodes in a given community (`graph/graph.go` lines 211–218). In `ComputeModularity`, this is not called directly, but any algorithm calling it per-community per-iteration (e.g., Louvain) will produce O(|V|) per community lookup, O(|V|²) total.
- Files: `graph/graph.go`
- Cause: No inverted index from community ID to node list.
- Improvement path: Build a `map[int][]NodeID` inverted index from the partition before iterating, or pre-compute per-community strengths during the `ComputeModularity` loop.

**`ComputeModularity` calls `g.Nodes()` which allocates a new slice every call:**
- Problem: `g.Nodes()` (`graph/graph.go` lines 79–85) allocates and returns a new `[]NodeID` slice on each call. `ComputeModularity` iterates this once, but in a hot Louvain loop this allocation is unnecessary.
- Files: `graph/modularity.go` line 41, `graph/graph.go` line 79`
- Cause: No iterator pattern; map range is not exposed.
- Improvement path: Add a `RangeNodes(func(NodeID))` callback method to avoid allocation, or accept that allocation cost is acceptable at this scale.

## Fragile Areas

**`Subgraph` correctness depends on canonical edge key deduplication:**
- Files: `graph/graph.go` lines 151–196
- Why fragile: The `seen` map uses `[2]NodeID{lo, hi}` canonical keys to avoid double-counting undirected edges. This logic is subtle and interacts with the adjacency list structure. Any future change to how undirected edges are stored (e.g., storing only one direction) will silently break `Subgraph` correctness.
- Safe modification: Any change to undirected storage must audit `Subgraph` and its test in `graph/graph_test.go` lines 146–164.
- Test coverage: One test (`TestSubgraph`) covers a minimal 4-node path; no self-loop or weighted subgraph tests exist.

**`testdata` package is inside `graph/` but has a separate package declaration:**
- Files: `graph/testdata/karate.go`
- Why fragile: The `testdata` directory is used as a Go package (`package testdata`) rather than the Go-idiomatic `testdata/` convention (which would be ignored by the build system for non-test files). This means it is compiled into the production binary when the `graph` package is imported, bloating binary size with 78 hardcoded edges.
- Safe modification: Move to a `_testdata` internal test helper, use `go:embed`, or restrict to `_test.go` files only.
- Test coverage: Used by `modularity_test.go` and `registry_test.go`.

## Scaling Limits

**In-memory adjacency map only:**
- Current capacity: Entire graph held in `map[NodeID][]Edge` in process memory.
- Limit: Practical limit is ~10M nodes / ~100M edges on a typical server before GC pressure becomes severe.
- Scaling path: No disk-backed, streaming, or chunked graph representation exists. A future Louvain implementation will need to hold community assignments and aggregated graphs simultaneously, further increasing memory pressure.

## Missing Critical Features

**No community detection algorithm implemented yet:**
- Problem: The repository is named `community-detection` and the branch is `feat/community-detection`, but no algorithm (Louvain, Girvan-Newman, Label Propagation, etc.) exists. Only the data structures and modularity scoring are present.
- Blocks: Any end-to-end use of the library; no phases beyond phase 01 have been executed.

**No error returns from any public function:**
- Problem: All public functions (`AddNode`, `AddEdge`, `Register`, `Clone`, `Subgraph`, etc.) return no errors. Invalid inputs (e.g., negative weights, duplicate edges) are silently accepted or produce undefined behavior.
- Files: `graph/graph.go`, `graph/registry.go`
- Blocks: Production-grade input validation; integration with callers that expect error-based contracts.

## Test Coverage Gaps

**No tests for `WeightToComm` or `CommStrength` on directed graphs:**
- What's not tested: Both helper methods are only tested on undirected graphs in `graph/graph_test.go` lines 166–189.
- Files: `graph/graph.go` lines 199–218
- Risk: Directed graph behavior (one-way edge weights) could silently produce wrong modularity values in directed community detection.
- Priority: Medium

**No tests for `Clone` preserving edge data correctly:**
- What's not tested: `TestClone` only verifies node count and total weight. It does not verify that adjacency lists in the clone are independent (mutating a cloned edge slice does not affect the original).
- Files: `graph/graph_test.go` lines 124–144
- Risk: Shallow copy bug in `Clone` would go undetected.
- Priority: Medium

**No tests for `Subgraph` with self-loops or directed graphs:**
- What's not tested: `TestSubgraph` uses an undirected path graph with no self-loops.
- Files: `graph/graph_test.go` lines 146–164
- Risk: Self-loop handling in `Subgraph` (line 190 guard) and directed subgraph correctness are untested.
- Priority: Medium

**No `registry_test.go` coverage for `NodeRegistry` bulk registration or concurrent access documentation:**
- What's not tested: Bulk registration patterns; the documented "not safe for concurrent use" contract has no test or race-detector coverage.
- Files: `graph/registry_test.go`
- Risk: Consumers could unknowingly use `NodeRegistry` concurrently and encounter data races.
- Priority: Low

---

*Concerns audit: 2026-03-29*
