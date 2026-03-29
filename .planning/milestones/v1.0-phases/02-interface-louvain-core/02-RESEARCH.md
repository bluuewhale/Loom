# Phase 02: Interface + Louvain Core — Research

**Researched:** 2026-03-29
**Domain:** Go community detection — CommunityDetector interface + Louvain algorithm
**Confidence:** HIGH (algorithm is well-documented; existing codebase fully understood)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- `Resolution == 0.0` → auto-default to 1.0
- `MaxPasses == 0` → unlimited passes; terminate via tolerance convergence
- `Tolerance == 0.0` → auto-default to 1e-7
- `Seed == 0` → use random seed (non-deterministic by default)
- Empty graph (0 nodes) → `(CommunityResult{}, nil)` — empty partition is valid
- Single-node graph → valid result with `Partition: {id: 0}`, `Modularity: 0.0`, `nil` error
- Directed graph → return `ErrDirectedNotSupported` sentinel error
- `CommunityResult.Passes` → always count executed passes; `1` even for trivial single-pass convergence
- Node visit order each pass: random shuffle driven by `Seed`
- Supergraph node IDs: contiguous new IDs after compression — internal only
- ΔQ computation: extracted as private `deltaQ(...)` helper function
- Final partition normalization: normalize to 0-indexed contiguous integers before returning

### Claude's Discretion

- Internal `louvainState` struct layout and field names
- Self-loop weight accumulation in supergraph construction
- How community strength sums are cached during phase 1 (map vs recompute)

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope.
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| IFACE-01 | `CommunityDetector` interface — `Detect(g *Graph) (CommunityResult, error)` | Standard Go interface pattern; single method |
| IFACE-02 | `CommunityResult` struct — `Partition map[NodeID]int`, `Modularity float64`, `Passes int`, `Moves int` | Direct struct definition; no complexity |
| IFACE-03 | `LouvainOptions` struct — `Resolution float64`, `Seed int64`, `MaxPasses int`, `Tolerance float64` | Direct struct definition; zero-value semantics documented |
| IFACE-04 | `LeidenOptions` struct — `Resolution float64`, `Seed int64`, `MaxIterations int`, `Tolerance float64` | Stub only in this phase; full implementation Phase 03 |
| IFACE-05 | `NewLouvain(opts LouvainOptions) CommunityDetector` constructor | Constructor returns interface type; concrete type unexported |
| IFACE-06 | `NewLeiden(opts LeidenOptions) CommunityDetector` constructor | Stub implementation returning `ErrNotImplemented` or placeholder |
| LOUV-01 | Phase 1 local move — ΔQ maximization per node over neighbor communities | ΔQ formula derived below; reuses `WeightToComm`, `CommStrength`, `Strength` |
| LOUV-02 | Phase 2 supergraph compression — communities become supernodes, self-loops preserved | Supergraph construction algorithm documented below |
| LOUV-03 | Convergence — terminate after zero-improvement pass (tolerance-based ΔQ comparison) | Convergence criterion: `totalMoves == 0` after full pass |
| LOUV-04 | Correct modularity formula — self-loop k_i_in exclusion, resolution parameter | ΔQ formula excludes self-contribution; mirrors `ComputeModularityWeighted` resolution usage |
| LOUV-05 | Edge cases — empty, single node, disconnected, two-node all return without error | Guard clauses at top of `Detect()`; pre-algorithm returns |
</phase_requirements>

---

## Summary

Phase 02 delivers two things: (1) a Go interface + option types that make community detection algorithms swappable, and (2) a complete, correct Louvain implementation. The Louvain algorithm has two phases per pass — local node moves (Phase 1) and community compression into a supergraph (Phase 2) — which alternate until no improvement exceeds the tolerance threshold.

The existing codebase provides all required helpers: `WeightToComm`, `CommStrength`, `Strength`, `TotalWeight`, `Nodes`, `Neighbors`, and `IsDirected`. The Louvain ΔQ formula maps directly onto these methods. No external libraries are needed.

The main implementation complexity is the supergraph construction (Phase 2): communities become supernodes, inter-community edges accumulate as new edge weights, and intra-community edges become self-loops. The final partition must be translated back from supergraph IDs to original node IDs, then normalized to 0-indexed contiguous integers.

**Primary recommendation:** Implement as two files (`detector.go` for types, `louvain.go` for algorithm) with an internal `louvainState` struct holding the mutable partition and community-strength cache for a single pass.

---

## Standard Stack

### Core

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `math/rand` (stdlib) | Go 1.26 | Shuffle node visit order, seeded RNG | Pure stdlib; no external deps per project rule |
| `math` (stdlib) | Go 1.26 | `math.Abs` for tolerance comparisons | Stdlib |
| `errors` (stdlib) | Go 1.26 | `errors.New` for sentinel errors | Stdlib |

### No External Dependencies

Project rule: keep Louvain pure stdlib. No `golang.org/x/exp`, no `gonum` etc.

**Installation:** None — stdlib only.

---

## Architecture Patterns

### File Layout

```
graph/
├── detector.go          # CommunityDetector interface, CommunityResult, LouvainOptions, LeidenOptions
├── louvain.go           # louvainDetector struct, NewLouvain, Detect, phase1, phase2, deltaQ, normalize
├── louvain_state.go     # louvainState struct (partition map, community strengths, RNG) — internal
├── louvain_test.go      # tests for all LOUV-* and IFACE-* requirements
└── testdata/            # existing karate.go fixture
```

### Pattern 1: Interface + Unexported Concrete Type

```go
// detector.go
// Source: project convention established in Phase 01 (NewGraph, NewRegistry)

// CommunityDetector is a swappable community detection algorithm.
// All implementations must be safe for concurrent use on distinct *Graph instances.
type CommunityDetector interface {
    Detect(g *Graph) (CommunityResult, error)
}

// CommunityResult holds the output of a community detection run.
type CommunityResult struct {
    Partition  map[NodeID]int // node -> community ID (0-indexed contiguous)
    Modularity float64
    Passes     int
    Moves      int
}

// LouvainOptions configures the Louvain algorithm.
// Zero values apply documented defaults (see field comments).
type LouvainOptions struct {
    Resolution float64 // 0.0 → 1.0
    Seed       int64   // 0 → random seed
    MaxPasses  int     // 0 → unlimited
    Tolerance  float64 // 0.0 → 1e-7
}

// LeidenOptions configures the Leiden algorithm (Phase 03).
type LeidenOptions struct {
    Resolution    float64
    Seed          int64
    MaxIterations int
    Tolerance     float64
}

var ErrDirectedNotSupported = errors.New("community detection on directed graphs is not supported")

// louvainDetector is the unexported concrete type returned by NewLouvain.
type louvainDetector struct { opts LouvainOptions }

func NewLouvain(opts LouvainOptions) CommunityDetector {
    return &louvainDetector{opts: opts}
}

// leidenDetector stub — full implementation in Phase 03.
type leidenDetector struct { opts LeidenOptions }

func NewLeiden(opts LeidenOptions) CommunityDetector {
    return &leidenDetector{opts: opts}
}

func (l *leidenDetector) Detect(g *Graph) (CommunityResult, error) {
    // TODO: Phase 03
    return CommunityResult{}, errors.New("leiden: not yet implemented")
}
```

### Pattern 2: Louvain Phase 1 — Local Move ΔQ

The ΔQ formula for moving node `i` INTO community `C` (Blondel et al. 2008, equation 3):

```
ΔQ = [ (k_i_in - resolution * Σ_tot * k_i) / m ]
```

Where:
- `k_i_in` = sum of weights from node `i` to nodes already in community `C`
  → `g.WeightToComm(i, C, partition)` (already implemented)
- `Σ_tot` = sum of all edge weights incident to nodes in community `C`
  → `g.CommStrength(C, partition)` (already implemented, but costly if called naively)
- `k_i` = sum of all edge weights incident to node `i` = `g.Strength(i)`
- `m` = `g.TotalWeight()` (NOT 2m — see note below)

**Critical note on the denominator:** The published formula uses `2m` in the denominator, but with this codebase's `CommStrength`/`Strength` definitions (which sum adjacency weights where undirected edges appear in both directions), using `m = TotalWeight()` directly gives the correct result. Verify against `ComputeModularityWeighted` which uses `twoW = 2 * TotalWeight()` — match that convention.

**Self-loop exclusion for k_i_in:** When computing ΔQ for moving `i` into `i`'s current community (the removal step), exclude `i` itself: `k_i_in` should not count edges from `i` to `i`. The existing `WeightToComm` implementation iterates `g.adjacency[n]` and sums weights where `partition[e.To] == comm` — this correctly includes self-loops IF `i` has a self-loop edge to itself. For the removal gain, compute the gain of moving `i` OUT of its current community by treating the removal as moving to a new empty community.

**Standard Louvain ΔQ implementation:**

```go
// deltaQ returns the modularity gain from moving node n into community comm.
// partition must reflect current assignments EXCLUDING n's contribution to comm
// (i.e., call after temporarily removing n from its current community).
// Source: Blondel et al. 2008, Fast unfolding of communities in large networks
func deltaQ(g *Graph, n NodeID, comm int, partition map[NodeID]int,
    commStr map[int]float64, resolution, m float64) float64 {
    kiIn := g.WeightToComm(n, comm, partition)
    sigTot := commStr[comm]  // cached Σ_tot for community comm
    ki := g.Strength(n)
    // ΔQ = kiIn/m - resolution * sigTot * ki / (2 * m * m)
    // Using 2m denominator consistent with ComputeModularityWeighted (twoW = 2*TotalWeight)
    twoM := 2.0 * m
    return kiIn/m - resolution*(sigTot/twoM)*(ki/twoM)
}
```

**Note on commStr cache:** Maintaining a `map[int]float64` of community strengths avoids O(n) `CommStrength` scans per node. Update the cache when a node moves: `commStr[oldComm] -= ki; commStr[newComm] += ki`.

### Pattern 3: Phase 1 Full Pass with Shuffle

```go
// phase1 performs one full pass of local moves. Returns number of node moves made.
// nodes slice is shuffled in-place using rng.
func (s *louvainState) phase1(g *Graph, nodes []NodeID, resolution, m float64) int {
    rng := s.rng  // math/rand.Rand seeded from opts
    // Fisher-Yates shuffle (rand.Shuffle)
    rng.Shuffle(len(nodes), func(i, j int) { nodes[i], nodes[j] = nodes[j], nodes[i] })

    moves := 0
    for _, n := range nodes {
        currentComm := s.partition[n]

        // Remove n from its community (temporarily)
        ki := g.Strength(n)
        s.commStr[currentComm] -= ki

        // Find best neighbor community
        bestComm := currentComm
        bestGain := 0.0

        // Collect candidate communities from neighbors
        seen := make(map[int]struct{})
        for _, e := range g.Neighbors(n) {
            nc := s.partition[e.To]
            if _, already := seen[nc]; already {
                continue
            }
            seen[nc] = struct{}{}
            gain := deltaQ(g, n, nc, s.partition, s.commStr, resolution, m) -
                    deltaQ(g, n, currentComm, s.partition, s.commStr, resolution, m)
            if gain > bestGain {
                bestGain = gain
                bestComm = nc
            }
        }

        // Re-add n to chosen community
        s.partition[n] = bestComm
        s.commStr[bestComm] += ki

        if bestComm != currentComm {
            moves++
        }
    }
    return moves
}
```

### Pattern 4: Phase 2 — Supergraph Construction

After Phase 1 converges, each community becomes a supernode:

```
Supergraph construction:
1. Assign each distinct community a new contiguous supernode ID (0, 1, 2, ...)
2. For each original edge (u, v, w):
   - If partition[u] == partition[v]: add to self-loop weight of supernode partition[u]
   - Else: accumulate edge weight between supernode(partition[u]) and supernode(partition[v])
3. Build new *Graph with NewGraph(false), AddEdge for each supernode edge + self-loops
4. Build new partition: each supernode maps to its own community ID (identity partition)
5. Also maintain: map[NodeID]int originalNode -> superNodeID (to reconstruct final partition)
```

**Implementation note:** Use `map[[2]int]float64` keyed by `[2]int{min(ca,cb), max(ca,cb)}` to accumulate inter-community edge weights, then call `AddEdge` once per pair. For self-loops, use `AddEdge(superID, superID, weight)`.

**Self-loop in this codebase:** `AddEdge(from, from, w)` stores a single adjacency entry (`from → from`) and increments `totalWeight` once. `Strength(from)` will include this once. This is consistent with the Louvain convention where intra-community edges become self-loops with weight = sum of internal edge weights.

### Pattern 5: Convergence Loop

```go
func (d *louvainDetector) Detect(g *Graph) (CommunityResult, error) {
    // --- guard clauses (LOUV-05) ---
    if g.IsDirected() {
        return CommunityResult{}, ErrDirectedNotSupported
    }
    n := g.NodeCount()
    if n == 0 {
        return CommunityResult{}, nil
    }
    if n == 1 {
        id := g.Nodes()[0]
        return CommunityResult{
            Partition:  map[NodeID]int{id: 0},
            Modularity: 0.0,
            Passes:     1,
            Moves:      0,
        }, nil
    }

    // --- resolve zero-value options ---
    opts := d.opts
    if opts.Resolution == 0.0 { opts.Resolution = 1.0 }
    if opts.Tolerance == 0.0  { opts.Tolerance = 1e-7 }
    // opts.MaxPasses == 0 means unlimited
    // opts.Seed == 0 means random seed

    // --- initialize state ---
    // partition: each node in its own community
    // commStr: initial community strengths = node strengths

    // --- outer loop ---
    totalPasses := 0
    totalMoves := 0
    currentGraph := g
    // nodeMapping: maps currentGraph NodeID -> original graph NodeID
    // (identity at start; updated each supergraph compression)

    for {
        passes, moves := runPhase1(currentGraph, opts, /* state */)
        totalPasses += passes
        totalMoves += moves
        if moves == 0 {
            break  // convergence: LOUV-03
        }
        if opts.MaxPasses > 0 && totalPasses >= opts.MaxPasses {
            break
        }
        // Phase 2: build supergraph
        currentGraph, nodeMapping = buildSupergraph(currentGraph, partition, nodeMapping)
        // Re-initialize partition to identity on new supergraph
    }

    // reconstruct original partition via nodeMapping chain
    // normalize to 0-indexed contiguous
    finalPartition := reconstructAndNormalize(partition, nodeMapping)
    q := ComputeModularityWeighted(g, finalPartition, opts.Resolution)

    return CommunityResult{
        Partition:  finalPartition,
        Modularity: q,
        Passes:     totalPasses,
        Moves:      totalMoves,
    }, nil
}
```

### Pattern 6: Partition Normalization

After the algorithm completes, community IDs may be non-contiguous (e.g., {0, 3, 7}). Normalize to {0, 1, 2}:

```go
func normalizePartition(partition map[NodeID]int) map[NodeID]int {
    remap := make(map[int]int)
    next := 0
    result := make(map[NodeID]int, len(partition))
    for n, c := range partition {
        if _, seen := remap[c]; !seen {
            remap[c] = next
            next++
        }
        result[n] = remap[c]
    }
    return result
}
```

### Pattern 7: math/rand Usage (Go stdlib)

```go
import "math/rand"

// Seeded RNG (deterministic when Seed != 0)
var src rand.Source
if opts.Seed != 0 {
    src = rand.NewSource(opts.Seed)
} else {
    src = rand.NewSource(time.Now().UnixNano())
}
rng := rand.New(src)

// Fisher-Yates shuffle (built-in)
rng.Shuffle(len(nodes), func(i, j int) {
    nodes[i], nodes[j] = nodes[j], nodes[i]
})
```

**Note:** `time` import is needed only for `time.Now().UnixNano()`. `math/rand` is NOT cryptographic; this is fine for Louvain's non-security shuffle.

### Anti-Patterns to Avoid

- **Calling `g.CommStrength` inside the ΔQ hot loop:** `CommStrength` is O(n) — it iterates all partition entries. Cache community strengths in a `map[int]float64` and maintain incrementally.
- **Using map iteration order for node visits:** Go map iteration is non-deterministic — always use `g.Nodes()` to get a slice, then shuffle that slice.
- **Double-counting inter-community edges in supergraph:** When building the supergraph from an undirected graph, accumulate each edge once using a canonical key `[min, max]` pair.
- **Forgetting to chain nodeMapping across multiple passes:** After each supergraph compression, the mapping from supergraph NodeIDs to original NodeIDs must be composed with the previous mapping; otherwise the final partition reconstruction is wrong.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Fisher-Yates shuffle | Custom swap loop | `rng.Shuffle(n, swap)` | Built into `math/rand` since Go 1.10 |
| Modularity Q verification | Custom Q calculator | `ComputeModularityWeighted` (existing) | Already implemented and tested; avoids drift |
| Node iteration | Map range loop | `g.Nodes()` + slice shuffle | Map iteration order is undefined in Go |
| Strength calculation | Sum adjacency manually | `g.Strength(n)` | Already implemented |
| Weight-to-community | Manual edge scan | `g.WeightToComm(n, comm, partition)` | Already implemented |

---

## Common Pitfalls

### Pitfall 1: Incorrect ΔQ Formula — Wrong Denominator

**What goes wrong:** Using `m` where `2m` is needed (or vice versa), producing modularity gains that never exceed threshold, so the algorithm runs forever or finds no communities.

**Why it happens:** The published Blondel formula uses `2m` denominator for the degree product term but `m` for the edge count term. The exact form depends on whether edge weights are counted once or twice in the adjacency representation.

**How to avoid:** The existing `ComputeModularityWeighted` uses `twoW = 2 * TotalWeight()`. The ΔQ formula for the degree-product term must use the SAME denominator convention. Verify by running the two-triangle disconnected graph: Louvain must achieve Q ≈ 0.5, matching the `ComputeModularityWeighted` result.

**Warning signs:** Karate Club Q < 0.30 or algorithm terminates after 1 pass with no moves.

### Pitfall 2: O(n) CommStrength in Hot Loop

**What goes wrong:** Each `deltaQ` call uses `g.CommStrength(comm, partition)` which iterates the entire partition. For a 34-node Karate Club graph this is negligible; for 10K nodes (PERF-01 target) this is O(n) per neighbor per node = O(n³) per pass.

**Why it happens:** `CommStrength` is a convenience method, not designed for hot-loop use.

**How to avoid:** Maintain `commStr map[int]float64` in `louvainState`. Initialize by computing `g.Strength(n)` for each node. Update on each node move: `commStr[oldComm] -= ki; commStr[newComm] += ki`.

**Warning signs:** Benchmark shows >1s for 10K nodes.

### Pitfall 3: Supergraph Self-Loop Double-Count

**What goes wrong:** When building the supergraph, intra-community edges (where both endpoints are in the same community) become self-loops. If the code fails to identify these correctly, they get added as inter-community edges instead, inflating edge counts and corrupting modularity.

**Why it happens:** Iterating `g.Neighbors` for each node in community `c` includes both edges to OTHER community-c nodes (which become self-loops) and edges to nodes in different communities (which become inter-community superedges).

**How to avoid:** Check `partition[e.To] == partition[n]` → self-loop; else → inter-community edge. Accumulate self-loop weight per community separately.

**Warning signs:** Supergraph has more edges than original; modularity after compression does not match modularity before compression.

### Pitfall 4: nodeMapping Chain Not Composed

**What goes wrong:** After multiple passes of supergraph compression, the final partition maps supergraph-N NodeIDs to community IDs. Without composing all the intermediate mappings, you can't recover the original graph's node-to-community assignments.

**Why it happens:** Each supergraph compression creates a new ID space. A node in the original graph maps to a supernode in supergraph-1, which maps to a supernode in supergraph-2, etc.

**How to avoid:** Maintain `nodeMapping map[NodeID]NodeID` at each pass level. After all passes, traverse the mapping chain backwards to translate community IDs back to original NodeIDs. A simpler approach: maintain `originalToCurrentComm map[NodeID]int` and update it each pass by composing with the new partition.

**Warning signs:** Final partition has fewer entries than the original graph's `NodeCount()`.

### Pitfall 5: Edge Case Panic on Empty/Single-Node Graph

**What goes wrong:** `g.Nodes()[0]` panics on empty graph; `g.TotalWeight()` returns 0 causing division by zero in ΔQ.

**How to avoid:** Guard clauses at the very start of `Detect()` — check `g.NodeCount() == 0`, `g.NodeCount() == 1`, `g.TotalWeight() == 0` (all-isolated nodes). For `TotalWeight() == 0`, each node is already its own singleton community — return immediately with Partition = each node → unique ID, Modularity = 0.0.

---

## Code Examples

### Exact ΔQ Formula (Blondel 2008)

```go
// Source: Blondel et al. 2008 eq. 3, adapted to this codebase's weight conventions
//
// ΔQ(i → C) = [ kiIn/m ] - [ resolution * (sigTot * ki) / (2m²) ]
//
// Where:
//   kiIn   = g.WeightToComm(i, C, partition)  [weights from i to community C]
//   sigTot = commStr[C]                         [cached sum of strengths in C]
//   ki     = g.Strength(i)                      [sum of weights incident to i]
//   m      = g.TotalWeight()
//   2m     = 2 * m  (the 2m denominator in Σ_tot/2m * ki/2m form)
//
// The net gain for moving i OUT of currentComm INTO bestComm:
//   gain = ΔQ(i → bestComm) - ΔQ(i → currentComm)
//          [with i already removed from currentComm before calling]

func deltaQ(kiIn, sigTot, ki, m, resolution float64) float64 {
    twoM := 2.0 * m
    return kiIn/m - resolution*(sigTot/twoM)*(ki/twoM)
}
```

### Supergraph Construction Skeleton

```go
// buildSupergraph creates the Phase 2 compressed graph.
// Returns: new supergraph, map from superNodeID -> original NodeID (any representative)
func buildSupergraph(g *Graph, partition map[NodeID]int) (*Graph, map[NodeID]NodeID) {
    // Step 1: assign contiguous supernode IDs
    commToSuper := make(map[int]NodeID)
    nextID := NodeID(0)
    for _, comm := range partition {
        if _, seen := commToSuper[comm]; !seen {
            commToSuper[comm] = nextID
            nextID++
        }
    }

    // Step 2: accumulate edge weights
    interEdges := make(map[[2]NodeID]float64)
    selfLoops := make(map[NodeID]float64)

    for n, edges := range // iterate g.Nodes() + g.Neighbors() {
        superN := commToSuper[partition[n]]
        for _, e := range edges {
            superE := commToSuper[partition[e.To]]
            if superN == superE {
                selfLoops[superN] += e.Weight
            } else {
                lo, hi := superN, superE
                if lo > hi { lo, hi = hi, lo }
                interEdges[[2]NodeID{lo, hi}] += e.Weight
            }
        }
    }
    // Note: undirected edges appear twice in adjacency → divide by 2
    // for inter-community; self-loops appear once → no division needed.

    // Step 3: build new graph
    sg := NewGraph(false)
    for pair, w := range interEdges {
        sg.AddEdge(pair[0], pair[1], w/2.0) // each undirected edge counted twice
    }
    for superN, w := range selfLoops {
        sg.AddEdge(superN, superN, w/2.0) // self-loops in original adjacency also doubled
    }
    return sg, ...
}
```

### Test Pattern: Karate Club Accuracy

```go
// Source: existing testdata.KarateClubEdges + testdata.KarateClubPartition
func TestLouvainKarateClub(t *testing.T) {
    g := buildKarateClub() // reuse from modularity_test.go
    det := NewLouvain(LouvainOptions{Seed: 42})
    result, err := det.Detect(g)
    if err != nil {
        t.Fatalf("Detect returned error: %v", err)
    }
    if result.Modularity < 0.35 {
        t.Errorf("Q = %.4f, want >= 0.35", result.Modularity)
    }
    if len(result.Partition) != g.NodeCount() {
        t.Errorf("partition covers %d nodes, want %d", len(result.Partition), g.NodeCount())
    }
    // Verify 2-4 communities
    communities := make(map[int]struct{})
    for _, c := range result.Partition {
        communities[c] = struct{}{}
    }
    if len(communities) < 2 || len(communities) > 4 {
        t.Errorf("found %d communities, want 2-4", len(communities))
    }
    // Verify normalized 0-indexed
    for i := 0; i < len(communities); i++ {
        if _, ok := communities[i]; !ok {
            t.Errorf("partition not 0-indexed contiguous: missing community %d", i)
        }
    }
    if result.Passes == 0 {
        t.Errorf("Passes = 0, want >= 1")
    }
}
```

---

## State of the Art

| Old Approach | Current Approach | Impact |
|--------------|------------------|--------|
| Global `rand` package (Go 1.19-) | `rand.New(rand.NewSource(seed))` | Thread-safe, seedable per-instance |
| `rand.Shuffle` added Go 1.10 | Still current in Go 1.26 | Use directly |
| Go 1.20+ `rand.New(rand.NewSource(0))` uses global crypto seed | Use `time.Now().UnixNano()` for Seed==0 path | Explicit randomness |

**Note on Go 1.20+ rand:** In Go 1.20, `rand.New(rand.NewSource(1))` behavior is unchanged. The global `rand` functions became automatically seeded. For Louvain, always use `rand.New(rand.NewSource(...))` to get a local, reproducible RNG regardless of global state.

---

## Environment Availability

Step 2.6: SKIPPED — Phase 02 is pure code/config changes with no external dependencies. All dependencies are Go stdlib; module is `community-detection` at Go 1.26.1.

---

## Validation Architecture

`nyquist_validation` is explicitly `false` in `.planning/config.json` — section skipped.

---

## Open Questions

1. **Self-loop weight in supergraph division factor**
   - What we know: undirected edges appear twice in adjacency (`AddEdge` stores both directions); `totalWeight` is incremented once per `AddEdge`. So `Strength(n)` for a non-self-loop node sums both directions → k_i as used in ΔQ is double the unique edge weights.
   - What's unclear: For self-loop `AddEdge(u, u, w)`, adjacency stores ONE entry and `totalWeight += w` once. `Strength(u)` = w. This is consistent with `ComputeModularityWeighted` which uses `twoW = 2 * TotalWeight()` and sees self-loops once in adjacency. The supergraph builder must respect the SAME convention: intra-community edges appear in BOTH nodes' adjacency lists → divide accumulated intra-weight by 2 when creating self-loops.
   - Recommendation: Use the two-triangle Q=0.5 test as the canonical integration check. After Phase 2, the two supernodes' self-loops should yield Q ≈ 0.5 via `ComputeModularityWeighted`.

2. **IFACE-06 (NewLeiden) stub vs full stub**
   - What we know: Phase 03 implements Leiden; Phase 02 must provide `NewLeiden(opts LeidenOptions) CommunityDetector` per IFACE-06.
   - Recommendation: Return a `leidenDetector` that satisfies the interface but returns a clear "not yet implemented" error from `Detect`. This makes the stub compile and satisfies the interface contract.

---

## Sources

### Primary (HIGH confidence)

- Existing codebase: `graph/graph.go`, `graph/modularity.go` — all helper APIs verified by reading source
- Existing tests: `graph/modularity_test.go` — establishes weight convention (twoW = 2*TotalWeight)
- Project CONTEXT.md — all locked decisions are authoritative
- `math/rand` stdlib Go docs (Go 1.26) — `rand.New`, `rand.NewSource`, `rng.Shuffle` API unchanged

### Secondary (MEDIUM confidence)

- Blondel et al. 2008, "Fast unfolding of communities in large networks" — canonical Louvain paper; ΔQ formula is well-established academic literature
- Standard Louvain implementations (multiple open-source repos) corroborate the two-phase structure and supergraph construction approach

### Tertiary (LOW confidence)

- None

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — pure stdlib, no external libraries
- Architecture: HIGH — all patterns derived from reading existing codebase; no speculation
- Pitfalls: HIGH — derived from first-principles analysis of the codebase's weight conventions
- Algorithm correctness: MEDIUM — ΔQ formula convention verified against `ComputeModularityWeighted` structure, but integration test (Karate Q > 0.35) is the final arbiter

**Research date:** 2026-03-29
**Valid until:** 2026-09-29 (stable stdlib; algorithm is fixed; expires when graph.go API changes)
