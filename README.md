# loom

High-performance community detection for Go. Louvain and Leiden algorithms, zero external dependencies, sub-100ms on 10K-node graphs.

Built for [GraphRAG](https://arxiv.org/abs/2404.16130) pipelines — where you need to cluster thousands of small graphs quickly and correctly.

## Features

- **Louvain** — greedy modularity optimization with multi-level supergraph compression (~48ms / 10K nodes)
- **Leiden** — BFS-refined variant that guarantees connected communities (~56ms / 10K nodes)
- Weighted and unweighted graphs, directed and undirected
- Newman-Girvan modularity with configurable resolution parameter
- `NodeRegistry` for string ↔ `NodeID` label mapping
- Pure Go stdlib — no CGo, no third-party deps
- Race-safe: all algorithms pass `-race`

## Install

```bash
go get github.com/bluuewhale/loom/graph
```

Requires Go 1.21+.

## Quick Start

```go
import "github.com/bluuewhale/loom/graph"

// Build a graph
g := graph.NewGraph(false) // undirected
g.AddEdge(0, 1, 1.0)
g.AddEdge(1, 2, 1.0)
g.AddEdge(2, 0, 1.0)
g.AddEdge(3, 4, 1.0)
g.AddEdge(4, 5, 1.0)
g.AddEdge(5, 3, 1.0)
g.AddEdge(2, 3, 0.1) // weak inter-cluster link

// Detect communities
det := graph.NewLouvain(graph.LouvainOptions{Seed: 42})
result, err := det.Detect(g)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Modularity: %.4f\n", result.Modularity)
for node, community := range result.Partition {
    fmt.Printf("  node %d → community %d\n", node, community)
}
```

### Using string labels

```go
reg := graph.NewRegistry()
g := graph.NewGraph(false)

alice := reg.Register("alice")
bob   := reg.Register("bob")
carol := reg.Register("carol")

g.AddEdge(alice, bob, 1.0)
g.AddEdge(bob, carol, 1.0)

det := graph.NewLeiden(graph.LeidenOptions{Seed: 42})
result, _ := det.Detect(g)

for node, community := range result.Partition {
    if label, ok := reg.Name(node); ok {
        fmt.Printf("  %s → community %d\n", label, community)
    }
}
```

## API

### Graph

```go
func NewGraph(directed bool) *Graph
func (g *Graph) AddEdge(from, to NodeID, weight float64)
func (g *Graph) AddNode(id NodeID, weight float64)
```

### Louvain

```go
func NewLouvain(opts LouvainOptions) CommunityDetector

type LouvainOptions struct {
    Seed       int64
    MaxPasses  int
    Tolerance  float64
    Resolution float64
}
```

### Leiden

```go
func NewLeiden(opts LeidenOptions) CommunityDetector

type LeidenOptions struct {
    Seed          int64
    MaxIterations int
    Tolerance     float64
    Resolution    float64
    NumRuns       int // multi-run best-Q selection (default 3 when Seed=0)
}
```

### Result

```go
type CommunityResult struct {
    Partition  map[NodeID]int // node → community ID
    Modularity float64
    Passes     int            // convergence iterations
    Moves      int            // total node moves
}
```

### Modularity

```go
func ComputeModularity(g *Graph, partition map[NodeID]int) float64
func ComputeModularityWeighted(g *Graph, partition map[NodeID]int, resolution float64) float64
```

### NodeRegistry

```go
func NewRegistry() *NodeRegistry
func (r *NodeRegistry) Register(name string) NodeID
func (r *NodeRegistry) Name(id NodeID) (string, bool)
func (r *NodeRegistry) ID(name string) (NodeID, bool)
func (r *NodeRegistry) Len() int
```

## Performance

Benchmarks on Apple M4 (arm64), undirected Barabasi-Albert graphs.

| Graph size | Library | Algorithm | Time | Communities | Notes |
|------------|---------|-----------|------|-------------|-------|
| 1K nodes | **loom** | Louvain | ~5.4ms | — | multi-level supergraph compression |
| 1K nodes | **loom** | Leiden | ~5.4ms | — | BFS-refined, connected communities |
| 1K nodes | python-louvain¹ | Louvain | ~91ms | — | |
| 10K nodes | **loom** | Louvain | ~63ms | ~22 | **~46x vs Python, ~37x vs gonum** |
| 10K nodes | **loom** | Leiden | ~65ms | ~22 | |
| 10K nodes | gonum² | Louvain | ~2.3s | ~22 | `community.Modularize`, general-purpose |
| 10K nodes | go-louvain³ | Louvain (1 pass) | ~10ms | ~4,300 | single phase — not quality-comparable |
| 10K nodes | leiden-go⁴ | Leiden | N/A | N/A | infinite loop bug on large graphs |
| 10K nodes | python-louvain¹ | Louvain | ~2,889ms | — | |

¹ `scripts/compare.py`: **python-louvain 0.16** (`community.best_partition`, `random_state=42`) + networkx 3.6. Install: `pip install networkx python-louvain`

² `scripts/go-compare/`: **gonum v0.17** (`community.Modularize`). Correct implementation; loom trades generality for tight inner loop + `sync.Pool` reuse.

³ `github.com/ledyba/go-louvain`: runs only a single Louvain pass with no supergraph compression — produces ~4,300 communities vs loom's ~22 on the same graph.

⁴ `github.com/vsuryav/leiden-go`: skipped — `refinePartition` sets `improved=true` unconditionally, causing an infinite loop on large random graphs.

Both loom algorithms use `sync.Pool` for internal state reuse — safe for concurrent use across goroutines.

## Accuracy

Validated on standard benchmark graphs:

| Dataset | Nodes | Edges | Louvain NMI | Leiden NMI |
|---------|-------|-------|-------------|------------|
| Karate Club | 34 | 78 | 0.83 | 0.72 |
| Political Books | 105 | 441 | 1.000 | 1.000 |
| College Football | 115 | 613 | 1.000 | 1.000 |

NMI (Normalized Mutual Information) measures partition quality against ground-truth labels. Higher is better; 1.0 is perfect.

## Testing

```bash
# All tests
go test ./graph

# With race detector
go test -race ./graph

# Benchmarks
go test -bench=. ./graph

# Verbose
go test -v ./graph
```

## GraphRAG Example

A common GraphRAG pipeline clusters a document similarity graph and uses each community as a context window:

```go
import "github.com/bluuewhale/loom/graph"

// 1. Build a similarity graph over document chunks.
//    Edge weight = cosine similarity; omit edges below threshold.
g := graph.NewGraph(false)
g.AddEdge(0, 1, 0.92)
g.AddEdge(1, 2, 0.87)
g.AddEdge(0, 2, 0.80)
g.AddEdge(3, 4, 0.95)
g.AddEdge(4, 5, 0.88)

// 2. Detect communities. Leiden guarantees connected communities —
//    each community becomes a coherent context window.
det := graph.NewLeiden(graph.LeidenOptions{Seed: 42})
result, err := det.Detect(g)
if err != nil {
    log.Fatal(err)
}

// 3. Group chunk IDs by community for LLM summarization.
clusters := make(map[int][]int)
for chunkID, comm := range result.Partition {
    clusters[comm] = append(clusters[comm], int(chunkID))
}
fmt.Printf("found %d communities, Q=%.4f\n", len(clusters), result.Modularity)
```

For online pipelines where the graph evolves incrementally, use warm-start to re-detect communities without starting from scratch:

```go
// Initial detection.
det := graph.NewLouvain(graph.LouvainOptions{Seed: 42})
result, _ := det.Detect(g)

// After adding/removing edges, re-use the prior partition as a seed.
// Warm-start converges in fewer passes when topology changes are small.
det2 := graph.NewLouvain(graph.LouvainOptions{
    Seed:             42,
    InitialPartition: result.Partition,
})
result2, _ := det2.Detect(updatedGraph)
```

## When to use Louvain vs Leiden

Use **Louvain** when speed matters most and you can tolerate occasional disconnected communities. It is slightly faster and simpler.

Use **Leiden** when community connectivity is a hard requirement. Leiden's BFS refinement step guarantees each detected community is a connected subgraph — important for GraphRAG chunking where disconnected communities produce incoherent context windows.

## Project structure

```
graph/
  graph.go          — Graph, NodeID, Edge, adjacency list
  detector.go       — CommunityDetector interface and options
  modularity.go     — ComputeModularity, ComputeModularityWeighted
  registry.go       — NodeRegistry
  louvain.go        — Louvain algorithm
  louvain_state.go  — louvainState with sync.Pool
  leiden.go         — Leiden algorithm
  leiden_state.go   — leidenState with sync.Pool
  testdata/         — Karate Club, Football, Political Books fixtures
  *_test.go         — Unit tests, benchmarks, race tests
```

## Status

v1.0 — all 24 requirements shipped. See [`.planning/ROADMAP.md`](.planning/ROADMAP.md) for the full milestone history.
