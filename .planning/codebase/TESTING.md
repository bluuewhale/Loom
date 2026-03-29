# Testing Patterns

**Analysis Date:** 2026-03-29

## Test Framework

**Runner:**
- Go standard library `testing` package — no third-party test runner
- Config: none (uses `go test ./...`)
- No `testify`, `gomock`, or other assertion libraries

**Assertion Library:**
- None — raw `t.Errorf` / `t.Fatalf` with inline formatting strings

**Run Commands:**
```bash
go test ./...              # Run all tests
go test -v ./...           # Verbose output
go test -run TestFoo ./... # Run specific test
go test -bench=. ./...     # Run benchmarks
go test -cover ./...       # Coverage report
```

## Test File Organization

**Location:** Co-located with source files in the same package directory
- `graph/graph_test.go` — tests for `graph/graph.go`
- `graph/modularity_test.go` — tests for `graph/modularity.go`
- `graph/registry_test.go` — tests for `graph/registry.go`

**Package declaration:** `package graph` (white-box testing — same package, access to unexported internals if needed)

**Fixture data location:** `graph/testdata/karate.go` — separate `testdata` package for large static datasets

**Naming:**
- Test functions: `Test<TypeOrBehavior><Case>` — `TestAddEdgeUndirected`, `TestModularityKarateClub`
- Benchmark functions: `Benchmark<Subject>` — `BenchmarkComputeModularityKarate`
- Helper functions: lowercase camelCase — `floatEq`, `containsEdge`, `containsNode`, `buildKarateClub`, `karatePartition`

## Test Structure

**Suite Organization:**
```go
// TestWeightToComm verifies WeightToComm sums weights to nodes in a community.
// Path 0-1-2, partition {0:0, 1:0, 2:1}
func TestWeightToComm(t *testing.T) {
    g := NewGraph(false)
    g.AddEdge(0, 1, 1.5)
    g.AddEdge(1, 2, 2.5)
    partition := map[NodeID]int{0: 0, 1: 0, 2: 1}

    floatEq(t, g.WeightToComm(1, 0, partition), 1.5) // edge 1->0
    floatEq(t, g.WeightToComm(1, 1, partition), 2.5) // edge 1->2
}
```

**Patterns:**
- Each test function has a doc comment naming its subject and describing the scenario/invariant
- Setup: construct minimal graph inline (no shared fixtures across tests except Karate Club)
- Assertion: call helper or inline `t.Errorf`/`t.Fatalf`
- `t.Fatal` used for fatal precondition failures (nil check, constructor failure)
- `t.Error` used for non-fatal assertion failures (allows multiple assertions per test)

## Mocking

**Framework:** None — no mocking library present
**Approach:** No mocking needed; all dependencies are pure value types (`*Graph`, `map[NodeID]int`)
**What to Mock:** Not applicable in current codebase
**What NOT to Mock:** Graph construction — always build real `*Graph` instances in tests

## Fixtures and Factories

**Karate Club fixture (`graph/testdata/karate.go`):**
```go
// package testdata
var KarateClubEdges = [][2]int{ ... }         // 78 undirected edges
var KarateClubPartition = map[int]int{ ... }  // ground-truth community partition
```

**Test helper functions (in test files, package-level):**
```go
// buildKarateClub creates the Karate Club graph from testdata.
func buildKarateClub() *Graph {
    g := NewGraph(false)
    for _, e := range testdata.KarateClubEdges {
        g.AddEdge(NodeID(e[0]), NodeID(e[1]), 1.0)
    }
    return g
}

// karatePartition converts int-keyed partition to NodeID-keyed.
func karatePartition() map[NodeID]int {
    p := make(map[NodeID]int, len(testdata.KarateClubPartition))
    for k, v := range testdata.KarateClubPartition {
        p[NodeID(k)] = v
    }
    return p
}
```

**Inline mini-graphs:** For unit tests, graphs are constructed inline with 2–6 nodes; no shared setup functions for simple cases.

## Assertion Helpers

Shared float comparison helper defined at top of `graph/graph_test.go`:
```go
func floatEq(t *testing.T, got, want float64) {
    t.Helper()
    if math.Abs(got-want) > 1e-9 {
        t.Errorf("got %v, want %v", got, want)
    }
}
```

Shared collection helpers:
```go
func containsEdge(t *testing.T, edges []Edge, to NodeID, weight float64) { ... }
func containsNode(t *testing.T, ids []NodeID, id NodeID) { ... }
```

All helpers call `t.Helper()` so failure lines point to the call site, not the helper.

## Table-Driven Tests

Used for edge case suites with multiple related scenarios:
```go
func TestModularityEdgeCases(t *testing.T) {
    tests := []struct {
        name      string
        buildFunc func() (*Graph, map[NodeID]int)
        wantQ     float64
        tolerance float64
    }{
        { name: "complete K5 all same community", ... },
        { name: "ring 6 nodes two halves", ... },
        { name: "disconnected two triangles", ... },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            g, p := tc.buildFunc()
            got := ComputeModularity(g, p)
            // assertion
        })
    }
}
```

Table entries use `buildFunc func() (*Graph, map[NodeID]int)` closures to keep construction inline.

## Benchmarks

```go
func BenchmarkComputeModularityKarate(b *testing.B) {
    g := buildKarateClub()
    p := karatePartition()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ComputeModularity(g, p)
    }
}
```

Pattern: build fixture outside timer, call `b.ResetTimer()`, then loop `b.N` times.

## Coverage

**Requirements:** Not enforced — no coverage threshold in CI or Makefile
**Approach:** Tests cover happy paths, boundary conditions, and degenerate inputs (empty graph, single node, self-loop)

## Test Types

**Unit Tests:**
- All current tests are unit tests
- Scope: single function/method per test
- Graph construction is inline; no I/O or external dependencies

**Integration Tests:** Not present

**E2E Tests:** Not present

## Numeric Tolerance Patterns

Two distinct tolerance levels are used:
- `1e-9` — tight epsilon for exact mathematical results (single community = 0.0, disconnected triangles = 0.5)
- `0.01` to `0.02` — loose tolerance for empirical reference values (Karate Club Q ≈ 0.371)

Always use `math.Abs(got-want) > tol` pattern, never `got != want` for float comparisons.

## Test Comment Style

Each test function has a doc comment:
```go
// TestAddEdgeUndirected verifies undirected edge storage and TotalWeight.
func TestAddEdgeUndirected(t *testing.T) { ... }
```

Complex tests include scenario comments explaining the expected values with inline math:
```go
// Triangle: 0-1-2; Subgraph([1,2]) should have 2 nodes, 1 edge.
```
