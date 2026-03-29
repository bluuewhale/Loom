# loom

High-performance community detection for Go. Louvain and Leiden algorithms, zero external dependencies, sub-100ms on 10K-node graphs.

Built for [GraphRAG](https://arxiv.org/abs/2404.16130) pipelines — where you need to cluster thousands of small graphs quickly and correctly.

## Features

- **Louvain** — greedy modularity optimization with multi-level supergraph compression (~48ms / 10K nodes)
- **Leiden** — BFS-refined variant that guarantees connected communities (~57ms / 10K nodes)
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
reg := graph.NewNodeRegistry()
g := graph.NewGraph(false)

alice := reg.Add("alice")
bob   := reg.Add("bob")
carol := reg.Add("carol")

g.AddEdge(alice, bob, 1.0)
g.AddEdge(bob, carol, 1.0)

det := graph.NewLeiden(graph.LeidenOptions{Seed: 42})
result, _ := det.Detect(g)

for node, community := range result.Partition {
    fmt.Printf("  %s → community %d\n", reg.Label(node), community)
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
    Seed       int64
    MaxPasses  int
    Tolerance  float64
    Resolution float64
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
func NewNodeRegistry() *NodeRegistry
func (r *NodeRegistry) Add(label string) NodeID
func (r *NodeRegistry) Label(id NodeID) string
func (r *NodeRegistry) ID(label string) (NodeID, bool)
```

## Performance

Benchmarks run on standard hardware, undirected graphs with random structure:

| Graph size | Algorithm | Time |
|------------|-----------|------|
| 10K nodes  | Louvain   | ~48ms |
| 10K nodes  | Leiden    | ~57ms |

Both algorithms use `sync.Pool` for internal state reuse — safe for concurrent use across goroutines.

## Accuracy

Validated on standard benchmark graphs:

| Dataset | Nodes | Edges | Louvain NMI | Leiden NMI |
|---------|-------|-------|-------------|------------|
| Karate Club | 34 | 78 | 0.65+ | 0.716 |
| Political Books | 105 | 441 | — | — |
| College Football | 115 | 613 | — | — |

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
