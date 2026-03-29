# Coding Conventions

**Analysis Date:** 2026-03-29

## Naming Patterns

**Files:**
- Snake-case not used; files named after their primary type/concept: `graph.go`, `registry.go`, `modularity.go`
- Test files co-located with source: `graph_test.go`, `registry_test.go`, `modularity_test.go`
- Test fixture data in `graph/testdata/` subdirectory: `karate.go`

**Types:**
- PascalCase for exported types: `NodeID`, `Edge`, `Graph`, `NodeRegistry`
- Type aliases via `type NodeID int` pattern for type-safe primitives

**Functions and Methods:**
- PascalCase for all exported functions and methods: `NewGraph`, `AddEdge`, `ComputeModularity`
- Constructor functions named `New<Type>`: `NewGraph(directed bool) *Graph`, `NewRegistry() *NodeRegistry`
- Receiver variable is a single lowercase letter matching the type: `g *Graph`, `r *NodeRegistry`

**Variables:**
- camelCase for local variables: `nodeSet`, `twoW`, `intraWeight`, `commStats`
- Short single-letter locals for tight loops: `c`, `s`, `w`, `e`, `id`
- Struct field names: camelCase unexported (`directed`, `nodes`, `adjacency`, `totalWeight`), PascalCase exported (`To`, `Weight`)

**Packages:**
- All production code lives in package `graph`
- Test fixture data lives in package `testdata` under `graph/testdata/`
- Module name matches repo directory: `community-detection`

## Code Style

**Formatting:**
- Standard `gofmt` formatting is assumed (Go convention)
- No explicit linter config file present; standard Go tooling only

**Linting:**
- No `.golangci.yml` or similar config detected; standard `go vet` expected

## Import Organization

**Order (Go standard):**
1. Standard library: `"math"`, `"testing"`
2. Internal packages: `"community-detection/graph/testdata"`

**No external dependencies** — `go.mod` declares only the Go version (1.26.1), no third-party modules.

## Error Handling

**Patterns:**
- No `error` return values used in the current codebase
- Invalid/missing inputs handled via sentinel returns: `(0, false)` for `ID()`, `("", false)` for `Name()`
- No-op semantics for idempotent operations: `AddNode` on an existing node is a silent no-op
- Missing nodes in partition maps default to community `-1` (singleton) in `ComputeModularity`
- Boundary returns for degenerate inputs: `ComputeModularity` returns `0.0` when graph has no edges

## Logging

**Framework:** None — production code uses no logging
**Test logging:** `t.Logf(...)` used for informational output in tests (e.g., Karate Club Q value)

## Comments

**When to Comment:**
- Every exported type, function, and method has a doc comment beginning with the identifier name
- Comments explain *why* and *how*, not just *what*: semantics, edge cases, complexity
- Multi-line comments for complex invariants (e.g., `Graph` struct comment explains undirected edge storage)
- Inline comments for non-obvious logic: `// Auto-create nodes if not present`, `// totalWeight counts each distinct edge once`

**Doc Comment Style:**
- First sentence: `// <Identifier> <verb phrase>.` — matches godoc convention
- Block-level comments explain algorithmic formulas inline: `// Formula: Q = Σ_c [ ... ]`

## Function Design

**Size:** Functions are small and focused; longest functions (`Subgraph`, `ComputeModularityWeighted`) are ~40 lines
**Parameters:** Prefer explicit typed parameters; no variadic or option-struct patterns yet
**Return Values:** Named return values not used; multi-value returns used for (value, bool) lookup pattern

## Module Design

**Exports:** Only types and functions needed by callers are exported; struct fields are unexported
**Barrel Files:** Not used; each file exposes its own exports directly
**Testdata package:** Fixture data is a separate `testdata` package under the main package directory, following Go convention for test fixtures

## Struct Design

- Internal state always unexported (`nodes map[NodeID]float64`, `adjacency map[NodeID][]Edge`)
- Struct literals with field names always used (not positional) in constructors
- `make(map[K]V)` with capacity hints where size is known: `make(map[NodeID]float64, len(g.nodes))`
