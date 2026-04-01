# Coding Conventions

**Analysis Date:** 2026-04-01

## Language & Runtime

**Go 1.26** (single module, no external dependencies).
All production code and tests live in `graph/` package (`package graph`).
Test files use `package graph` (white-box), giving tests full access to unexported symbols.

---

## Naming Patterns

**Files:**
- One primary concern per file: `louvain.go`, `leiden.go`, `modularity.go`, `ego_splitting.go`, `registry.go`, `graph.go`
- State/pool helpers use `_state` suffix: `louvain_state.go`, `leiden_state.go`
- Test files mirror source: `louvain_test.go`, `leiden_test.go`, `ego_splitting_test.go`
- Build-tag pair: `race_test.go` / `norace_test.go`

**Types:**
- Exported interfaces: noun + role suffix — `CommunityDetector`, `OverlappingCommunityDetector`, `OnlineOverlappingCommunityDetector`
- Unexported implementations: algorithm + `Detector` — `louvainDetector`, `leidenDetector`, `egoSplittingDetector`
- State structs: algorithm + `State` — `louvainState`, `leidenState`
- Result/option structs: noun + descriptor — `CommunityResult`, `OverlappingCommunityResult`, `LouvainOptions`, `LeidenOptions`, `EgoSplittingOptions`

**Functions:**
- Constructors: `New` prefix — `NewLouvain`, `NewLeiden`, `NewEgoSplitting`, `NewOnlineEgoSplitting`, `NewGraph`, `NewRegistry`
- Pool acquire/release pairs: `acquireLouvainState` / `releaseLouvainState`, `acquireLeidenState` / `releaseLeidenState`
- Unexported algorithm phases: `phase1`, `buildSupergraph`, `refinePartition`, `reconstructPartition`, `normalizePartition`
- Helpers named for what they build: `buildEgoNet`, `buildPersonaGraph`, `buildPersonaGraphIncremental`, `computeAffected`, `mapPersonasToOriginal`

**Variables:**
- Short identifiers in hot loops: `n`, `m`, `ki`, `dq`, `comm`, `superN`
- Derived constants as local vars: `twoM`, `kiOverTwoM`, `kiOverTwoM`
- Exported errors: `Err` prefix — `ErrDirectedNotSupported`, `ErrEmptyGraph`

---

## Code Style

**Formatting:** Standard `gofmt`. No custom formatter config detected.

**Linting:** No `.golangci.yml` or linter config present. Idiomatic Go patterns throughout.

---

## Import Organization

**Order (standard Go convention):**
1. Standard library: `math`, `slices`, `sync`, `time`, `errors`, `runtime`, `sort`
2. Internal package (test files only): `github.com/bluuewhale/loom/graph/testdata`

No path aliases. No third-party dependencies anywhere.

---

## Guard Clause Pattern

All `Detect` methods open with identical guard clauses before any computation:

```go
if g.IsDirected() {
    return CommunityResult{}, ErrDirectedNotSupported
}
if g.NodeCount() == 0 {
    return CommunityResult{}, nil
}
if g.NodeCount() == 1 {
    node := g.Nodes()[0]
    return CommunityResult{Partition: map[NodeID]int{node: 0}, Modularity: 0.0, Passes: 1, Moves: 0}, nil
}
if g.TotalWeight() == 0 {
    // All nodes disconnected: each in own community.
    ...
}
```

This block is copy-pasted verbatim in `louvain.go:12-43` and `leiden.go:21-51`.

---

## Error Handling

**Patterns:**
- Package-level sentinel errors via `errors.New`: `ErrDirectedNotSupported`, `ErrEmptyGraph`
- Errors propagated as direct returns: `return CommunityResult{}, err`
- No error wrapping (`fmt.Errorf %w`) — sentinel errors only
- Multi-run Leiden intentionally discards `lastErr` when at least one run succeeds (documented in comment at `leiden.go:87-90`)
- Tests assert sentinel errors with `errors.Is`: `errors.Is(err, ErrDirectedNotSupported)`

---

## Performance Conventions

### Object Pooling via sync.Pool

Both state types use `sync.Pool` to reduce GC pressure:

```go
var louvainStatePool = sync.Pool{
    New: func() any {
        return &louvainState{
            partition:     make(map[NodeID]int),
            commStr:       make(map[int]float64),
            neighborBuf:   make(map[NodeID]float64),
            neighborDirty: make([]NodeID, 0, 64),
            candidateBuf:  make([]int, 0, 64),
        }
    },
}
```

Callers always pair acquire with deferred release:
```go
state := acquireLouvainState(currentGraph, seed)
defer releaseLouvainState(state)
```

### Dirty-List Buffer Reset (avoids full map clear per node)

```go
// In phase1(), per-node neighbor accumulation:
for _, k := range state.neighborDirty {
    delete(state.neighborBuf, k)
}
state.neighborDirty = state.neighborDirty[:0]
// ... accumulate writes into neighborBuf, append keys to neighborDirty
```

Avoids O(map capacity) cost of `clear()` per node when only a small number of neighbors are touched.

### Pre-allocated Slice Capacity

All reusable buffers start with capacity 64:
```go
neighborDirty: make([]NodeID, 0, 64),
candidateBuf:  make([]int, 0, 64),
```

### Sorted Traversal for Determinism

`slices.Sort(nodes)` before `rng.Shuffle` ensures the RNG seed is the sole source of traversal randomness (map iteration order is intentionally non-deterministic in Go):
```go
nodes := g.Nodes()
slices.Sort(nodes)
state.rng.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })
```

### Insertion Sort for Small Candidate Slices

```go
// In phase1(): candidate count is bounded by node degree — small in practice.
for i := 1; i < len(candidates); i++ {
    for j := i; j > 0 && candidates[j] < candidates[j-1]; j-- {
        candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
    }
}
```

### Canonical Edge Key for Deduplication

Used consistently in `buildSupergraph`, `Subgraph`, `buildPersonaGraph`, and incremental rebuild:
```go
lo, hi := from, e.To
if lo > hi { lo, hi = hi, lo }
key := [2]NodeID{lo, hi}
if _, already := seen[key]; already { continue }
seen[key] = struct{}{}
```

### Parallel Ego-Net Detection

`buildPersonaGraph` in `ego_splitting.go:463-503` dispatches non-isolated ego-nets to `runtime.GOMAXPROCS(0)` workers. Ego-nets are built once before dispatch:
```go
workerCount := runtime.GOMAXPROCS(0)
// ... build all egoNets, store in nonEmptyJobs
go func() {
    for _, job := range nonEmptyJobs { jobCh <- egoNetJob{...} }
    close(jobCh)
}()
results := runParallelEgoNets(jobCh, localDetector, workerCount)
```

### isolatedOnly Fast-Path in Update

When all affected nodes are newly-added isolated nodes (no edges), `buildPersonaGraphIncremental` returns `isolatedOnly=true`, allowing `Update` to skip global Louvain entirely and assign singletons directly.

---

## Comments

**Doc comments:** Every exported type, function, and method has a Go doc comment.

**Mathematical formulas cited inline:**
```go
// ΔQ(comm) = kiIn/m - resolution*(sigTot/(2m))*(ki/(2m))
// Formula: Q = Σ_c [ intraWeight/twoW - resolution * (degSum/twoW)^2 ]
```

**Algorithm references:** Epasto, Lattanzi, Paes Leme 2017 (Ego Splitting) and Newman-Girvan modularity cited in function-level comments.

**Requirement IDs tracked in comments:**
```go
// (ONLINE-05), (ONLINE-06), (ONLINE-11), (EGO-CRIT-02), (PERF-02), (IG-2)
```

---

## Module Design

**Exports:**
- All constructors exported: `NewLouvain`, `NewLeiden`, `NewEgoSplitting`, `NewOnlineEgoSplitting`
- All interfaces exported: `CommunityDetector`, `OverlappingCommunityDetector`, `OnlineOverlappingCommunityDetector`
- Concrete types unexported — callers program to interfaces

**Unexported carry-forward fields on `OverlappingCommunityResult`:**
```go
personaOf        map[NodeID]map[int]NodeID
inverseMap       map[NodeID]NodeID
partitions       map[NodeID]map[NodeID]int
personaPartition map[NodeID]int
personaGraph     *Graph
```
Accessible only within package; passed through `Update()` for incremental recomputation.

**Barrel Files:** None — single `graph` package, flat structure.

**Testdata subpackage:** `graph/testdata/` exports `KarateClubEdges`, `FootballEdges`, `PolbooksEdges` and matching `*Partition` maps as package-level variables.

---

## Concurrency Contract

- `CommunityDetector` implementations: safe for concurrent use **on distinct `*Graph` instances**
- `sync.Pool` ensures state isolation between concurrent `Detect` calls
- `cloneDetector` creates per-worker detector copies so goroutine workers never share mutable RNG state
- `NodeRegistry`: explicitly **not safe** for concurrent use (documented)

---

## Code Duplication Hotspots

1. **Guard clauses**: Identical 4-case block copied verbatim in `louvain.go:12-43` and `leiden.go:21-51`
2. **`reset()` body**: `louvainState.reset` (`louvain_state.go:49-116`) and `leidenState.reset` (`leiden_state.go:53-121`) are near-identical; warm-start logic is fully duplicated
3. **Legacy constructors kept alongside pool**: `newLouvainState` (`louvain_state.go:120-147`) and `newLeidenState` (`leiden_state.go:127-155`) duplicate pool-path initialization for "backward-compatibility"
4. **Deduplicate + compact community IDs**: Identical ~30-line block in `Detect` (`ego_splitting.go:143-189`) and `Update` (`ego_splitting.go:278-319`)
5. **Edge-wiring loop**: The canonical `(lo,hi)` dedup + persona lookup loop written twice inside `buildPersonaGraphIncremental` — incremental patch path (`ego_splitting.go:836-873`) and full-rebuild fallback (`ego_splitting.go:882-919`)
6. **Graph builder test helpers**: `buildKarateClubLouvain` (`louvain_test.go:14-20`), `buildKarateClubLeiden` (`leiden_test.go:13-19`), and `buildKarateClub` (`modularity_test.go:11-17`) all build the identical graph

---

*Convention analysis: 2026-04-01*
