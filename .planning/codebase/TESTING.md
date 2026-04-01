# Testing Patterns

**Analysis Date:** 2026-04-01

## Test Framework

**Runner:** Go standard `testing` package (Go 1.26)
**Config:** No external test framework or config file — `go test ./graph/...` is sufficient

**Run Commands:**
```bash
go test ./graph/...                        # All tests
go test -run TestLouvain ./graph/...       # Filtered tests
go test -bench=. -benchmem ./graph/...    # All benchmarks with alloc reporting
go test -race ./graph/...                  # Race detector
go test -short ./graph/...                 # Skip slow performance tests
go test -count=5 -bench=. ./graph/...     # Multi-sample benchmark (used for bench-baseline.txt)
```

---

## Test File Organization

**Location:** Co-located with source in `graph/` — all files in `package graph` (white-box).

**Naming:**
- Unit/integration tests: `<subject>_test.go`
- Shared test helpers (unexported): `testhelpers_test.go`
- Build-tag pair for race detection flag: `race_test.go` / `norace_test.go`
- Specialized concern files: `accuracy_test.go`, `benchmark_test.go`, `leiden_numruns_test.go`

**Structure:**
```
graph/
├── graph_test.go           # Graph primitive unit tests
├── modularity_test.go      # ComputeModularity / ComputeModularityWeighted tests + benchmark
├── louvain_test.go         # Louvain Detect correctness tests
├── leiden_test.go          # Leiden Detect correctness tests
├── leiden_numruns_test.go  # Leiden NumRuns / multi-run behavior tests
├── detector_test.go        # Interface/constructor/zero-value tests
├── ego_splitting_test.go   # EgoSplitting Detect + Update + online invariant tests (largest file)
├── accuracy_test.go        # NMI accuracy + warm-start quality tests across fixtures
├── benchmark_test.go       # BA graph benchmarks + warm-start speedup + concurrent tests
├── testhelpers_test.go     # Shared helpers: nmi, uniqueCommunities, buildGraph, perturbGraph, cloneWithAdditions
├── race_test.go            # Builds with -race: const raceEnabled = true
└── norace_test.go          # Builds without -race: const raceEnabled = false
```

---

## Test Structure

**Suite Organization — table-driven where applicable:**
```go
tests := []struct {
    name      string
    buildFunc func() (*Graph, map[NodeID]int)
    wantQ     float64
    tolerance float64
}{...}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) { ... })
}
```

**Flat naming for single-case tests:**
```go
func TestLouvainKarateClub(t *testing.T) { ... }
func TestLouvainEmptyGraph(t *testing.T) { ... }
func TestLouvainSingleNode(t *testing.T) { ... }
```

**Setup pattern:**
```go
g := buildGraph(testdata.KarateClubEdges)  // from testhelpers_test.go
det := NewLouvain(LouvainOptions{Seed: 42})
res, err := det.Detect(g)
if err != nil { t.Fatalf("unexpected error: %v", err) }
```

**Logging results:** `t.Logf` used extensively to report Q, community count, NMI, and pass counts — visible with `-v`.

---

## Fixtures and Factories

**Real-world graph fixtures** in `graph/testdata/`:
- `testdata.KarateClubEdges` — Zachary's Karate Club, 34 nodes, 78 edges
- `testdata.FootballEdges` — NCAA Football, 115 nodes, 12 conferences
- `testdata.PolbooksEdges` — Political books, 105 nodes, 3 communities
- Matching `*Partition` maps for NMI/Omega ground-truth comparison

**Synthetic graph helpers** in `testhelpers_test.go`:
```go
func buildGraph(edges [][2]int) *Graph        // undirected graph from edge list
func perturbGraph(g *Graph, nRemove, nAdd int, seed int64) *Graph  // controlled perturbation for warm-start tests
func cloneWithAdditions(g *Graph, newNodes []NodeID, newEdges []DeltaEdge) *Graph  // online update benchmark setup
```

**Inline micro-graphs** in `ego_splitting_test.go`:
```go
func makeTriangle() *Graph { ... }     // 3-node complete graph
func makeStar(n int) *Graph { ... }    // star with n spokes
func makeBarbell() *Graph { ... }      // 4-node barbell (triangle + bridge)
```

**BA (Barabasi-Albert) synthetic graphs** in `benchmark_test.go`:
```go
var bench1K  *Graph  // init(): generateBA(1_000, 5, seed=42)
var bench10K *Graph  // init(): generateBA(10_000, 5, seed=42)
```
Initialized once at package load via `init()`. All benchmarks share the same pointer (read-only).

---

## Mocking

**Framework:** No mocking library. Test spies implemented manually.

**countingDetector** (in `ego_splitting_test.go:1166-1190`):
```go
type countingDetector struct {
    mu    sync.Mutex
    inner CommunityDetector
    count int
}
func (c *countingDetector) Detect(g *Graph) (CommunityResult, error) {
    c.mu.Lock(); c.count++; c.mu.Unlock()
    return c.inner.Detect(g)
}
```
Used by `TestUpdate_AffectedNodesOnly` to verify that `Update` calls `LocalDetector.Detect` exactly `len(affected)` times — not `g.NodeCount()` times.

**What to Mock:** Only `CommunityDetector` (via the `countingDetector` spy) when testing invocation counts. Never mock `*Graph` — always use real instances.

**What NOT to Mock:** `*Graph`, partition maps, or any data structures — tests always use real data.

---

## Accuracy / Quality Tests

**NMI (Normalized Mutual Information)** computed in `testhelpers_test.go:nmi()`:
```go
score := nmi(res.Partition, groundTruthPartition(testdata.KarateClubPartition))
if score < 0.7 { t.Errorf("NMI = %.4f, want >= 0.7", score) }
```
Used in: `TestLouvainKarateClubNMI` (NMI >= 0.7), `TestLouvainFootballNMI` (NMI >= 0.95), `TestLouvainPolbooksNMI` (NMI >= 0.95), `TestLeidenKarateClubAccuracy` (NMI >= 0.7), and Leiden Football/Polbooks variants. All in `accuracy_test.go` and `leiden_test.go`.

**Omega Index** for overlapping community accuracy (`omega.go:OmegaIndex`):
```go
omega := OmegaIndex(result, groundTruth)
if omega < 0.3 { t.Errorf(...) }  // lowered from 0.5 — documented ceiling for EgoSplitting
```
Used in `TestEgoSplittingOmegaIndex` across KarateClub (0.428), Football (0.821), Polbooks (0.467). Seed 101 identified as empirically best via sweep of seeds 1-200.

**Modularity bounds:** Most detection tests assert `res.Modularity > 0.35` (Karate Club) or `> 0.0` (other graphs).

---

## Performance Tests

### Benchmark Functions

All benchmarks follow the pattern: warmup call before `b.ResetTimer()`, then `b.ReportAllocs()`:

```go
func BenchmarkLouvain10K(b *testing.B) {
    det := NewLouvain(LouvainOptions{Seed: 1})
    det.Detect(bench10K)  // warmup: populate sync.Pool
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        det.Detect(bench10K)
    }
}
```

**Benchmark inventory** (all in `graph/benchmark_test.go` and `graph/ego_splitting_test.go`):

| Benchmark | Graph | Purpose |
|---|---|---|
| `BenchmarkLouvain1K` | BA 1K nodes | Go vs Python NetworkX comparison |
| `BenchmarkLeiden1K` | BA 1K nodes | Go vs Python NetworkX comparison |
| `BenchmarkLouvain10K` | BA 10K nodes | Primary regression target (<100ms/op) |
| `BenchmarkLeiden10K` | BA 10K nodes | Primary regression target (<100ms/op) |
| `BenchmarkLouvain10K_Allocs` | BA 10K nodes | Dedicated alloc tracking for benchstat |
| `BenchmarkLouvainWarmStart` | BA 10K perturbed | Warm-start speedup vs cold |
| `BenchmarkLeidenWarmStart` | BA 10K perturbed | Warm-start speedup vs cold |
| `BenchmarkEgoSplitting10K` | BA 10K nodes | EgoSplitting regression (<300ms/op) |
| `BenchmarkEgoSplittingUpdate1Node1Edge` | BA 10K + 1 node | Online Update incremental cost |
| `BenchmarkEgoSplittingUpdate2Edges` | BA 10K + 2 edges | Online Update incremental cost |
| `BenchmarkUpdate_EmptyDelta` | Triangle | Empty delta = 0 allocs |
| `BenchmarkComputeModularityKarate` | Karate Club | Modularity computation micro-bench |

### Baseline Numbers (bench-baseline.txt, Apple M4, arm64)

```
BenchmarkLouvain1K-10       ~5.4ms/op   ~2.33MB/op  5217 allocs/op
BenchmarkLeiden1K-10        ~5.5ms/op   ~2.58MB/op  7248 allocs/op
BenchmarkLouvain10K-10      ~68ms/op   ~18.7MB/op  48788 allocs/op
BenchmarkLeiden10K-10       ~73ms/op   ~21.1MB/op  66524 allocs/op
BenchmarkLouvainWarmStart-10 ~46ms/op  ~10.3MB/op  25754 allocs/op  (~1.5x faster than cold)
BenchmarkLeidenWarmStart-10  ~51ms/op  ~13.2MB/op  37130 allocs/op  (~1.4x faster than cold)
BenchmarkComputeModularityKarate-10  ~2-4µs/op   320B/op  3 allocs/op
```

Note: Louvain10K shows high variance (59ms to 92ms) across runs — variance is ~35%.

### Performance Assertion Tests (test functions that measure timing)

These tests call `testing.Benchmark()` internally to measure performance programmatically:

**`TestEgoSplitting10KUnder300ms`** (`benchmark_test.go:207`):
- Runs `BenchmarkEgoSplitting10K` via `testing.Benchmark()`
- Fails if `msPerOp > 500` (500ms regression guard; target is 300ms)
- Skips under `-short` and `-race`

**`TestLouvainWarmStartSpeedup`** (`benchmark_test.go:229`):
- Measures cold `BenchmarkLouvain10K` vs warm `BenchmarkLouvainWarmStart`
- Fails if `speedup < 1.2x`
- Skips under `-short`

**`TestLeidenWarmStartSpeedup`** (`benchmark_test.go:249`):
- Same pattern as Louvain variant, `speedup < 1.2x` threshold

**`TestEgoSplittingUpdateAllocSavings`** (`ego_splitting_test.go:1762`):
- Compares allocs between cold `BenchmarkEgoSplitting10K` and `BenchmarkEgoSplittingUpdate1Node1Edge`
- Fails if `allocSpeedup < 2.0x`
- Skips under `-short` and `-race`

---

## Concurrency Tests

**`TestConcurrentDetect`** (`benchmark_test.go:267`):
- 4 goroutines × 10 Detect calls on distinct `*Graph` instances
- Run with `-race` to catch data races (PERF-02)

**`TestEgoSplittingConcurrentDetect`** (`ego_splitting_test.go:459`):
- 4 goroutines × 5 EgoSplitting Detect calls on distinct instances (EGO-10)

**`TestEgoSplittingConcurrentUpdate`** (`ego_splitting_test.go:1646`):
- 8 goroutines × 3 sequential Updates, each goroutine has its own detector + graph + prior
- Run with `-race` to catch violations (ONLINE-13)

---

## Edge Case Coverage

**Graph topology edge cases tested:**
- Empty graph (0 nodes)
- Single node
- Two nodes + one edge
- Disconnected nodes (5 isolated)
- Complete graph K5
- Ring graph
- Two disconnected triangles
- Two triangles connected by bridge
- Triangle + isolated nodes
- Star topology (center + 5 leaves)
- Self-loop + normal edge
- Directed graph (error path)

**Warm-start edge cases** (`accuracy_test.go:TestWarmStartEdgeCases`):
- `EmptyGraph (CG-1a)`: InitialPartition on empty graph
- `SingleNode (CG-1b)`: InitialPartition on single-node graph
- `StaleKeys (CG-2)`: Full 34-node partition warm-starting a 14-node subgraph
- `CompleteMismatch (CG-3)`: All partition keys absent from graph (degenerates to cold)
- `Idempotent (CG-4)`: Seeding with already-converged partition → ≤2 passes, Moves=0

**Online Update edge cases:**
- Empty delta → prior returned unchanged, 0 allocs (`BenchmarkUpdate_EmptyDelta`)
- Nil carry-forward fields → fallback to full `Detect()`
- Multiple sequential updates maintaining PersonaID disjointness (ONLINE-11)
- Isolated node addition (fast-path: `isolatedOnly=true`)
- Directed graph error propagation

---

## Structural Invariant Testing

**`assertResultInvariants`** (`ego_splitting_test.go:1474`):
A reusable helper that checks three invariants on `OverlappingCommunityResult`:
1. Every node in `g` appears in `NodeCommunities` with ≥1 community
2. Every community index in `NodeCommunities` is in-bounds for `Communities`
3. `NodeCommunities ↔ Communities` bidirectional consistency

Called by `TestUpdateResultInvariants` across 6 delta scenarios (empty delta, isolated node addition, edge addition, multi-node batch, node+edge together, nil carry-forward fallback).

---

## Coverage Gaps

**Not tested:**
- `CommStrength` method on `Graph` (`graph.go:245`) — no direct unit test
- `deltaQ` standalone function (`louvain.go:261`) — tested indirectly via `phase1` but never directly
- `warmStartedDetector` with unknown detector type (default branch at `ego_splitting.go:355`) — unreachable in normal use but untested
- `BenchmarkEgoSplittingUpdate2Edges` has no corresponding performance assertion test (only a benchmark, no `TestEgoSplittingUpdate2EdgesSpeedup`)
- No test for `NodeRegistry` concurrent safety violation (documented as not safe, but no test confirming it races)
- `refinePartition` not tested in isolation — only tested end-to-end via Leiden `Detect`
- No fuzz tests despite graph operations (edge addition, partition building) being good candidates

**Accuracy gap documented in source:**
- `TestEgoSplittingOmegaIndex` threshold is 0.3 (lowered from 0.5 target) because EgoSplitting produces ~19 micro-communities on Karate Club. Root cause documented but not resolved. See `ego_splitting_test.go:396-455`.

---

## Test Utilities Reference

**`testhelpers_test.go`** functions (available to all test files in package):
- `nmi(p1, p2 map[NodeID]int) float64` — normalized mutual information
- `uniqueCommunities(partition map[NodeID]int) int` — count distinct communities
- `buildGraph(edges [][2]int) *Graph` — canonical graph builder
- `groundTruthPartition(gt map[int]int) map[NodeID]int` — type conversion helper
- `perturbGraph(g *Graph, nRemove, nAdd int, seed int64) *Graph` — seeded perturbation
- `cloneWithAdditions(g *Graph, ...) *Graph` — online benchmark setup helper

**`partitionToGroundTruth`** (in `ego_splitting_test.go`) — converts `map[int]int` to `[][]NodeID` for `OmegaIndex`.

---

*Testing analysis: 2026-04-01*
