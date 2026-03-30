# Stack Research — Ego Splitting Framework (v1.2)

**Domain:** Overlapping community detection via ego-net splitting (Go library)
**Researched:** 2026-03-30
**Confidence:** HIGH (Go stdlib patterns) / HIGH (integration with existing codebase)

---

## Core Verdict: Zero New Runtime Dependencies

The ego-splitting framework is fully implementable with stdlib + the existing `package graph`.
No external packages are needed or recommended. This section documents exactly which stdlib
packages and internal patterns cover each new capability.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go stdlib only | 1.26.1 | All algorithm code | Zero-dep posture is a library feature; every transitive dep taxes callers |
| `package graph` (internal) | v1.1 | Graph, NodeID, Edge, CommunityDetector, Louvain, Leiden | Reuse existing validated types; no duplication |
| `math/rand` | stdlib | Deterministic RNG for ego-net / persona graph construction | Already used in louvain_state.go with `rand.New(rand.NewSource(seed))` pattern |
| `slices` | stdlib 1.21+ | Sorted node iteration for determinism | Already used throughout; `slices.Sort` on `[]NodeID` |
| `sync` | stdlib | `sync.Pool` for egoState / personaState scratch buffers | Proven pattern in louvain_state.go and leiden_state.go |

### New Types Needed (all within `package graph`)

| Type | Kind | Purpose | Notes |
|------|------|---------|-------|
| `OverlappingCommunityDetector` | interface | Algorithm 3 entry point — mirrors `CommunityDetector` | Single method: `Detect(g *Graph) (OverlappingCommunityResult, error)` |
| `OverlappingCommunityResult` | struct | Return type holding multi-membership partitions | `Communities [][]NodeID` (each community is a node set); `NodeCommunities map[NodeID][]int` for reverse lookup |
| `EgoSplittingOptions` | struct | Configuration for ego-splitting algorithm | `LocalDetector CommunityDetector`, `GlobalDetector CommunityDetector`, `Seed int64` |
| `egoSplittingDetector` | struct (unexported) | Implements `OverlappingCommunityDetector` | Returned by `NewEgoSplitting(opts EgoSplittingOptions)` |

### Supporting Stdlib Packages

| Package | Purpose | When Used |
|---------|---------|-----------|
| `sync.Pool` | Reuse egoState scratch maps/slices across ego-net iterations | Algorithm 1 inner loop: one ego-net per node, N iterations per graph |
| `slices` (stdlib 1.21+) | Deterministic sorted iteration; sort persona node lists | Algorithm 1 & 2: node ordering for reproducibility |
| `maps` (stdlib 1.21+) | `maps.Clone` for partition copying | Algorithm 3: copy `CommunityResult.Partition` before backprojecting |
| `math/rand` | Seeded RNG for deterministic persona assignment when ties exist | Algorithm 2: deterministic behavior under fixed seed |
| `errors` | Sentinel errors (`ErrDirectedNotSupported` reuse) | All three algorithms: directed graphs unsupported |

### Development Tools (unchanged from v1.0/v1.1)

| Tool | Purpose | Notes |
|------|---------|-------|
| `go test -race` | Concurrent safety validation | Mandatory gate; egoSplitting must pass |
| `go test -bench` + `-benchmem` | 10K-node ~200-300ms performance target | Persona graph is 2-3x larger than input |
| `benchstat` | Statistical comparison of benchmark runs | `go install golang.org/x/perf/cmd/benchstat@latest` |
| `go tool pprof` | CPU/heap flamegraph during optimization | Bundled in toolchain |

---

## Integration Points with Existing `package graph`

### What Ego Splitting Reuses Directly

| Existing Symbol | How Ego Splitting Uses It |
|----------------|--------------------------|
| `Graph` | ego-net is a `*Graph` (via `g.Subgraph(neighbors)`); persona graph is also a `*Graph` |
| `g.Subgraph(nodeIDs []NodeID)` | Algorithm 1: extract ego-net for each node (excludes ego itself per paper) |
| `g.Nodes()` | Iterate all nodes to build ego-nets |
| `g.Neighbors(id)` | Get neighbor set for each ego |
| `NodeID` | Persona nodes are new `NodeID` values; need a fresh counter above `g.NodeCount()` |
| `CommunityDetector` interface | Algorithm 1 local detection + Algorithm 3 global detection — both are pluggable |
| `NewLouvain` / `NewLeiden` | Default detectors passed via `EgoSplittingOptions.LocalDetector` and `.GlobalDetector` |
| `CommunityResult.Partition` | Algorithm 1 output; used to assign persona IDs in Algorithm 2 |
| `ErrDirectedNotSupported` | Reuse sentinel; ego splitting also requires undirected input |

### What Must Be Added

| New Capability | Implementation Note |
|---------------|-------------------|
| Persona node ID allocation | Simple counter: `personaBase := NodeID(g.NodeCount())`, then `personaBase + offset`. No registry needed for internal use. |
| Persona-to-original mapping | `map[NodeID]NodeID` — persona node back to original node. Used in Algorithm 3 backprojection. |
| Original-to-personas mapping | `map[NodeID][]NodeID` — original node to its persona nodes. Used to determine overlapping membership. |
| Persona graph construction | `NewGraph(false)` then `AddEdge` for each original edge, routed through the persona that covers it (Algorithm 2 definition). |
| Overlapping result assembly | After global `CommunityDetector.Detect(personaGraph)`, group persona communities back by original NodeID. Multiple persona nodes for the same original in different communities = overlapping membership. |

---

## Algorithm-Specific Patterns

### Algorithm 1: Ego-Net Construction + Local Detection

```
for each node v in g:
    neighbors := g.Neighbors(v)            // []Edge
    neighborIDs := extract NodeIDs         // []NodeID (exclude v itself)
    egoNet := g.Subgraph(neighborIDs)      // *Graph — existing method
    localResult := localDetector.Detect(egoNet)   // CommunityResult
    store localResult.Partition as egoPartition[v]
```

Key: `g.Subgraph` already exists and handles undirected edge deduplication correctly.
The ego node itself is excluded from its ego-net (standard definition — neighbors only).

### Algorithm 2: Persona Graph Generation

```
personaGraph := NewGraph(false)           // undirected
personaOf := map[NodeID][]NodeID{}        // original -> list of persona nodes
originalOf := map[NodeID]NodeID{}         // persona -> original

for each node v:
    communities := distinct values in egoPartition[v]
    for each community c:
        p := allocate new NodeID
        personaGraph.AddNode(p, 1.0)
        personaOf[v] = append(personaOf[v], p)
        originalOf[p] = v

for each edge (u,v) in g:
    pu := persona of u that shares community with v in egoPartition[u]
    pv := persona of v that shares community with u in egoPartition[v]
    personaGraph.AddEdge(pu, pv, edge.Weight)
```

No new data structures needed beyond stdlib maps and the existing `Graph`.

### Algorithm 3: Global Detection + Backprojection

```
globalResult := globalDetector.Detect(personaGraph)   // CommunityResult on persona graph
overlapping := map[NodeID][]int{}                      // original node -> community IDs

for personaNode, commID := range globalResult.Partition:
    orig := originalOf[personaNode]
    overlapping[orig] = append(overlapping[orig], commID)

// deduplicate per node (a node may appear in same community via multiple personas)
// assemble OverlappingCommunityResult
```

---

## Concurrency Design

The ego-splitting algorithm has an embarrassingly parallel inner loop (Algorithm 1): each
node's ego-net detection is independent. However, for v1.2, start with a **sequential
implementation** to establish correctness first. Parallelism can be added in a follow-on
phase if benchmarks show the 200-300ms target is not met.

If parallelism is added:

| Pattern | Use Case |
|---------|---------|
| Worker pool with `sync.WaitGroup` | Parallel ego-net detection (Algorithm 1) |
| Pre-allocated `[]CommunityResult` indexed by node position | Avoid map contention when storing per-ego results |
| `sync.Pool` for egoState | Reuse scratch buffers across goroutines |

The existing `CommunityDetector` implementations (Louvain, Leiden) are already documented as
"safe for concurrent use on distinct `*Graph` instances" — this holds for parallel ego-net
detection since each ego-net is a distinct `*Graph`.

---

## Persona Graph Size Expectations

| Input Graph | Expected Persona Graph Size | Notes |
|-------------|---------------------------|-------|
| Karate Club (34n, 78e) | ~50-80 persona nodes | Low-overlap graphs expand minimally |
| Football (115n, 613e) | ~200-400 persona nodes | Moderate overlap |
| Polbooks (105n, 441e) | ~150-300 persona nodes | Moderate overlap |
| Synthetic 10K nodes | ~20K-30K persona nodes | 2-3x expansion is the expected range per paper |

The 200-300ms target for 10K nodes assumes 2-3x graph expansion. The persona graph's
community detection (Algorithm 3) dominates runtime. Using `NewLouvain` as default is
appropriate — it's the faster of the two (~48ms vs ~57ms at 10K).

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| Single `package graph` for all new types | New subpackage `graph/overlapping` | Only if the package grows large enough to warrant split (deferred) |
| `CommunityDetector` as LocalDetector and GlobalDetector | Hardcode Louvain internally | Never — the interface exists precisely to keep algorithms swappable |
| Sequential Algorithm 1 first | Parallel ego-net loop from the start | Prefer parallel only after correctness tests pass and benchmarks show need |
| `map[NodeID][]NodeID` for persona mapping | Separate registry type | Overkill; maps suffice given no string labels are involved |
| `[][]NodeID` for `OverlappingCommunityResult.Communities` | `map[int][][NodeID]` | Slice-of-slices is simpler to iterate and matches how callers consume communities |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| External ego-splitting libraries (Python igraph, networkx) | CGO/interop violates constraints; wrong language | Implement Algorithm 1-3 directly in Go |
| `gonum.org/v1/gonum/graph` | No overlapping community detection; heavyweight interface; transitive dep | Stay with custom `Graph` |
| `GOEXPERIMENT=arena` | Unstable experimental API; may be removed | `sync.Pool` |
| New persona `NodeID` > `math.MaxInt/2` range | Overflow risk if graph is very large | Use `NodeID(len(nodes) + offset)` with explicit overflow check |
| Storing persona graph in a separate package-level var | Race condition under concurrent `Detect` calls | Allocate fresh persona graph inside each `Detect` call |

---

## Go Version Features Used

| Feature | Go Version | Use in Ego Splitting |
|---------|-----------|----------------------|
| `slices.Sort` | 1.21+ | Sort `[]NodeID` for deterministic iteration in Algorithms 1-3 |
| `maps.Clone` | 1.21+ | Copy partition maps during backprojection |
| `clear(m)` builtin | 1.21+ | Reset pooled scratch maps (already used in louvain_state.go) |
| `sync.Pool` | 1.3+ | Scratch buffer reuse — already established pattern |
| Range over func (iterators) | 1.23+ | Optional: iterate ego-net neighbors without allocating a slice copy |

All features are available in Go 1.26.1 as required by go.mod.

---

## Installation

No new dependencies. The module file (`go.mod`) requires no changes.

```bash
# Verify module is clean after adding ego-splitting files
go mod tidy
go test -race ./graph/...
go test -bench=BenchmarkEgoSplitting -benchmem ./graph/...
```

---

## Sources

- Existing `graph/graph.go` — `Subgraph`, `Neighbors`, `Nodes`, `AddEdge`, `NewGraph` confirmed available
- Existing `graph/detector.go` — `CommunityDetector` interface, `CommunityResult` type confirmed
- Existing `graph/louvain_state.go` — `sync.Pool` + `clear()` + `slices.Sort` pattern confirmed
- Go 1.26.1 stdlib `slices`, `maps`, `sync`, `math/rand` — all available, no import changes needed
- Ego Splitting paper: Epasto, Gleich, Kumar (Google, 2017) "Ego-splitting Framework: from Non-Overlapping to Overlapping Clusters" — Algorithm 1/2/3 structure
- go.mod: `go 1.26.1`, zero external deps confirmed

---
*Stack research for: Ego Splitting Framework overlapping community detection (v1.2)*
*Researched: 2026-03-30*
