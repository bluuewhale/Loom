# Feature Landscape: Louvain + Leiden Community Detection

**Domain:** Graph community detection algorithms for a Go GraphRAG library
**Researched:** 2026-03-29
**Confidence:** HIGH (algorithm papers) / MEDIUM (Go-specific interface patterns)

---

## Table Stakes

Features callers expect. Missing = library feels incomplete or untrustworthy.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `CommunityDetector` interface | Single swap-in point; project requirement | Low | Must accept `*Graph`, return `Result` |
| Resolution parameter γ (float64) | Controls community granularity; every production implementation exposes this | Low | Default 1.0 = standard modularity |
| Random seed (int64) | Reproducibility is mandatory for debugging and testing | Low | 0 = use system entropy |
| Max iterations per pass (int) | Prevents infinite loops on adversarial graphs; standard safeguard | Low | Default 10 (louvain-igraph convention) |
| Final modularity Q in result | Callers need a quality signal; directly available since ComputeModularity exists | Low | Reuse existing `ComputeModularity` |
| Partition output `map[NodeID]int` | Already the project's canonical type; zero-conversion cost | Low | Matches existing `Partition` usage |
| Empty graph / no-edge graph handling | GraphRAG subgraphs can be degenerate; callers must not panic | Low | Return empty partition, Q=0 |
| Single-node graph handling | Common during dynamic subgraph construction in GraphRAG | Low | Return `{node: 0}`, Q=0 |
| Disconnected graph handling | Disconnected components are normal in sparse KG subgraphs | Medium | Each component treated independently |

---

## Algorithm Parameters (Specific Types)

### `LouvainOptions` struct

```go
type LouvainOptions struct {
    Resolution  float64 // γ — scales community size; default 1.0; range (0, ∞)
    MaxPasses   int     // outer passes (coarsening rounds); 0 = run until convergence
    MaxMoves    int     // inner moves per pass; default 10; 0 = unlimited
    Tolerance   float64 // delta-Q threshold to declare convergence; default 1e-7
    Seed        int64   // RNG seed for node visit order; 0 = non-deterministic
}
```

**Resolution γ:** Values > 1.0 → more, smaller communities. Values < 1.0 → fewer, larger communities. GraphRAG users need smaller communities (γ ≈ 1.0–2.0) to get coherent semantic chunks. [Source: louvain-igraph docs, Neptune Analytics docs]

**Tolerance:** Used in the local-moving convergence check: sum of Δ-modularity across all node moves in one pass < tolerance → stop. Production default is 1e-7. [Source: GVE-Louvain paper]

### `LeidenOptions` struct

```go
type LeidenOptions struct {
    Resolution    float64 // γ — same semantics as Louvain; default 1.0
    MaxIterations int     // full algorithm iterations (each = move+refine+aggregate); 0 = until convergence
    Tolerance     float64 // delta-Q convergence threshold; default 1e-7
    Seed          int64   // RNG seed; 0 = non-deterministic
}
```

Leiden does not need `MaxMoves` because its local-moving phase uses a faster queue-based visiting order that naturally bounds work per iteration. [Source: Traag et al. 2019 paper]

---

## `CommunityDetector` Interface Design

Recommended interface — minimal, swap-able, fits existing free-function style:

```go
// CommunityDetector runs a community detection algorithm on g and returns
// the partition and execution metadata. Implementations must be safe for
// concurrent calls on distinct *Graph values but need not be safe for
// concurrent calls on the same *Graph.
type CommunityDetector interface {
    Detect(g *Graph) (CommunityResult, error)
}

// CommunityResult is the output of a community detection run.
type CommunityResult struct {
    Partition  map[NodeID]int // node → community label (0-based, dense)
    Modularity float64        // final Q of the returned partition
    Passes     int            // number of coarsening passes completed
    Moves      int            // total node moves across all passes
}
```

**Rationale for error return:** Unlike `ComputeModularity` (pure math, no failure modes), `Detect` takes options that can be invalid (negative resolution, etc.). Returning an error is idiomatic Go and prevents silent misconfiguration.

**Why not return `[]map[NodeID]int` hierarchy?** The full dendrogram is rarely needed in GraphRAG — callers want the final best partition. Add a `DetectHierarchy` variant only if there is explicit demand. Keep the primary interface minimal.

**Concrete constructors:**

```go
func NewLouvain(opts LouvainOptions) CommunityDetector
func NewLeiden(opts LeidenOptions) CommunityDetector

// Zero-config convenience — uses defaults
func DefaultLouvain() CommunityDetector
func DefaultLeiden() CommunityDetector
```

**Interface pattern evidence:** Neo4j GDS, Graphology, and NetworkX all separate "get partition" from "get detailed execution info." The `CommunityResult` struct above merges both into one return to keep Go idiomatic (single return type, no optional second call). [Source: Neo4j GDS docs, Graphology docs]

---

## Output Shape Beyond `map[NodeID]int`

| Field | Type | Why Needed | When Useful |
|-------|------|-----------|-------------|
| `Partition` | `map[NodeID]int` | Primary output; community assignment | Always |
| `Modularity` | `float64` | Quality gate — callers compare runs or reject degenerate results | Always |
| `Passes` | `int` | Diagnostics — did it converge early or hit `MaxPasses`? | Debugging, tuning |
| `Moves` | `int` | Rough proxy for how much structure changed | Debugging |

**Not included (defer until explicitly needed):**

- Full dendrogram / `[]map[NodeID]int` — needed only for hierarchical community browsing; not a GraphRAG use case
- Per-level modularity slice — useful for multi-resolution analysis; add to `DetectHierarchy` later
- Community size distribution — derivable from `Partition` by caller; don't pollute the struct

---

## Differentiators

Features that distinguish this library from a naive implementation.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Leiden connected-community guarantee | Prevents downstream failures in GraphRAG where disconnected "communities" break summarization | High | Core Leiden advantage; the primary reason to implement it |
| Iterative Leiden convergence | Leiden proves convergence to γ-dense, subset-optimal partition when run iteratively | Medium | Run Leiden in a loop until partition unchanged |
| `use_lcc` option (largest connected component only) | Microsoft GraphRAG's own recommendation for sparse KG graphs; focuses clustering, filters noise | Medium | `UseLCC bool` in options — extract LCC, detect, map back |
| Concurrent-safe detector instances | Multiple goroutines each calling `Detect` on distinct graphs; matches real-time query model | Medium | Stateless detectors after construction; all state in local vars |
| Threshold scaling in Louvain | Start with high tolerance, reduce each pass — cuts runtime on large graphs | Medium | Evidence: GVE-Louvain paper shows significant speedup |

---

## Anti-Features

Do not build these; they add cost without value for this library.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Overlapping community detection (fuzzy membership) | Adds O(n²) complexity; GraphRAG does not use overlapping communities | Hard partition only |
| Distributed / multi-machine parallelism | Out of scope per PROJECT.md; adds operational complexity | Goroutine parallelism within one process |
| Graph I/O (load from file) | Out of scope per PROJECT.md | Caller constructs `*Graph` via existing API |
| Dynamic community detection (incremental updates) | Research-grade, unstable API; unnecessary for current use case | Re-run full detection on updated graph |
| Spectral methods (eigenvector-based) | Better at disconnected graphs but O(n³); incompatible with <100ms target on 10K nodes | Leiden handles disconnected via `use_lcc` |
| Auto-tuning resolution γ | Heuristics are fragile; callers in GraphRAG pipelines need deterministic behavior | Document γ semantics; let caller set it |

---

## Feature Dependencies

```
Graph (graph.go)           →  CommunityDetector.Detect
ComputeModularity          →  CommunityResult.Modularity
NodeRegistry (optional)    →  Graph construction (no dependency on detection)

LouvainDetector            →  CommunityDetector interface
LeidenDetector             →  CommunityDetector interface
Leiden                     →  NOT dependent on Louvain (independent implementation)

use_lcc option             →  requires connected-component extraction (new utility)
Benchmark fixtures         →  Karate Club (existing) + Football + Polbooks (new)
```

---

## Benchmark Graphs for Accuracy Validation

| Graph | Nodes | Edges | Communities | Why Use It |
|-------|-------|-------|-------------|-----------|
| Karate Club | 34 | 78 | 2 (ground-truth) | Already in codebase; baseline sanity check |
| College Football | 115 | 613 | 12 (conferences) | Medium size; well-known ground truth; Louvain canonical validation graph |
| Polbooks | 105 | 441 | 3 (liberal/neutral/conservative) | Easy to validate; clear 3-community structure |
| LFR synthetic (µ=0.1) | 1,000 | variable | ~10–50 | Controlled mixing parameter; measures algorithm sensitivity to noise |
| LFR synthetic (µ=0.3) | 1,000 | variable | ~10–50 | Harder benchmark; community boundaries less sharp |

**Accuracy metrics to assert in tests:**
- Normalized Mutual Information (NMI) ≥ 0.90 on Football with default parameters [MEDIUM confidence — varies by seed]
- Modularity Q ≥ 0.35 on Karate Club (already validated in Phase 01-02)
- All returned communities are connected (Leiden guarantee — assert in tests)

**Source:** Lancichinetti et al. 2008 (LFR benchmark paper), Newman datasets (Football, Polbooks). [Source: arxiv.org/abs/0805.4770, VLDB benchmarking study]

---

## Leiden vs Louvain: Key Differences That Matter for GraphRAG

### The Core Flaw Leiden Fixes

Louvain can produce **disconnected communities** — nodes assigned to the same community label despite having no path between them within that community. Traag et al. (2019) found up to 25% of Louvain communities badly connected and up to 16% fully disconnected in empirical tests.

**Why this matters for GraphRAG:** A community summary is built from all nodes in a community. A disconnected community conflates semantically unrelated entities, degrading summary quality and downstream retrieval accuracy. Leiden's connectivity guarantee prevents this failure mode entirely.

### Algorithmic Difference

| Property | Louvain | Leiden |
|----------|---------|--------|
| Phases per iteration | 2 (local-move, aggregate) | 3 (local-move, **refinement**, aggregate) |
| Community connectivity | Not guaranteed | Guaranteed connected |
| Convergence guarantee | Local optimum only | γ-dense + subset-optimal when iterated |
| Speed vs Louvain | Baseline | 20–150% faster despite extra phase |
| Reproducibility | Seed-dependent | Seed-dependent (same property) |

### Refinement Phase (Leiden's Key Innovation)

After local-moving, Leiden runs a refinement step that may split communities to ensure connectivity. The aggregation then uses the non-refined (coarser) partition to initialize the next iteration. This two-pointer approach is what makes Leiden faster while producing better results. [Source: Traag et al. Scientific Reports 2019]

### Recommendation

**Use Leiden as the default.** Expose Louvain as an alternative for users with existing pipelines that require it. For GraphRAG, the connected-community guarantee alone justifies the choice.

---

## Edge Cases Callers Will Hit in GraphRAG Contexts

### EG-01: Empty Graph (0 nodes, 0 edges)
- **Scenario:** Subgraph from a query that matched no entities
- **Expected behavior:** Return `CommunityResult{Partition: map[NodeID]int{}, Modularity: 0.0, Passes: 0, Moves: 0}`, no panic, no error
- **Implementation:** Guard at entry: `if g.Len() == 0 { return empty result, nil }`

### EG-02: Single-Node Graph
- **Scenario:** Entity extracted from document with no relationships
- **Expected behavior:** `{nodeID: 0}`, Q=0.0
- **Implementation:** Guard: `if g.Len() == 1 { return singleton partition, nil }`

### EG-03: Fully Disconnected Graph (N nodes, 0 edges)
- **Scenario:** NodeRegistry populated but no edges added yet
- **Expected behavior:** Each node is its own community; Q=0.0
- **Implementation:** Phase 1 of Louvain/Leiden will naturally produce singletons; verify Q formula returns 0 not NaN

### EG-04: Graph with One Giant Component + Many Singletons
- **Scenario:** Typical sparse KG — most entities connect to one cluster, but many are isolated
- **Expected behavior with `use_lcc=false`:** Singletons each form their own community; may inflate community count
- **Expected behavior with `use_lcc=true`:** Only LCC participates; singleton nodes get community label -1 or are excluded from partition
- **Implementation:** `UseLCC` flag extracts largest connected component, runs detection on it, maps results back; isolated nodes absent from Partition map

### EG-05: Two-Node Graph (One Edge)
- **Scenario:** Minimal subgraph — two entities, one relationship
- **Expected behavior:** Both nodes in same community; Q > 0
- **Implementation:** Louvain/Leiden naturally handles; verify no divide-by-zero in modularity

### EG-06: Negative or Zero Resolution
- **Scenario:** Misconfigured caller passes `Resolution: 0` or `Resolution: -1.0`
- **Expected behavior:** Return error: `errors.New("resolution must be > 0")`
- **Implementation:** Validate in `NewLouvain` / `NewLeiden` constructors, not in `Detect`

### EG-07: Very Dense Graph (Complete Graph K_n)
- **Scenario:** Unlikely in GraphRAG but possible in test; every node connected to every other
- **Expected behavior:** Single community (all nodes together) or split at resolution > 1; Q near 0 for single community on complete graph
- **Implementation:** Convergence should be fast (1 pass); no special handling needed

### EG-08: Graph with Self-Loops
- **Scenario:** Some knowledge graph extractors emit self-referential edges
- **Expected behavior:** Self-loops contribute to node strength but not modularity Q (standard convention)
- **Implementation:** `ComputeModularity` must skip self-loops in intra-community sum; verify this is already the case

---

## Sparse Graph Warning (GraphRAG-Specific)

Research (2025) found that on sparse KGs where average degree is constant and most nodes have low degree, Leiden-based communities are **inherently non-reproducible** across seeds, with exponentially many near-optimal partitions. This is not a bug — it reflects genuine ambiguity in the graph structure.

**Implication for API design:** Do not document Leiden as producing a "best" partition for sparse graphs. Document it as producing "a good partition" and recommend callers run multiple seeds and compare Q scores if reproducibility matters. [Source: Microsoft GraphRAG discussions, arxiv community detection review 2024]

---

## MVP Recommendation

Prioritize in this order:

1. `CommunityDetector` interface + `CommunityResult` struct (unblocks everything)
2. Leiden implementation (better algorithm; GraphRAG default; what Microsoft GraphRAG uses)
3. Edge case guards (EG-01 through EG-05)
4. Louvain implementation (secondary; needed for completeness and benchmark comparison)
5. Football + Polbooks benchmark fixtures (accuracy validation for both algorithms)
6. `UseLCC` option (important for production GraphRAG; defer until core algorithms validated)

**Defer to later phases:**
- Full dendrogram output — no expressed need
- LFR synthetic benchmark generation — adds a dependency or significant code; use static fixture instead
- `DetectHierarchy` variant — add only with concrete use case

---

## Sources

- [From Louvain to Leiden: guaranteeing well-connected communities (Traag et al., Scientific Reports 2019)](https://www.nature.com/articles/s41598-019-41695-z)
- [Leiden algorithm — Wikipedia](https://en.wikipedia.org/wiki/Leiden_algorithm)
- [GVE-Louvain: Fast Louvain in Shared Memory (arxiv)](https://arxiv.org/html/2312.04876v4)
- [louvain-igraph reference docs](https://louvain-igraph.readthedocs.io/en/latest/reference.html)
- [Neo4j GDS Louvain algorithm docs](https://neo4j.com/docs/graph-data-science/current/algorithms/louvain/)
- [Neo4j GDS Leiden algorithm docs](https://neo4j.com/docs/graph-data-science/current/algorithms/leiden/)
- [Graphology communities-louvain API](https://graphology.github.io/standard-library/communities-louvain.html)
- [NetworkX louvain_communities docs](https://networkx.org/documentation/stable/reference/algorithms/generated/networkx.algorithms.community.louvain.louvain_communities.html)
- [GraphRAG community detection concept (Microsoft)](https://www.mintlify.com/microsoft/graphrag/concepts/community-detection)
- [GraphRAG parameterless discussion (GitHub)](https://github.com/microsoft/graphrag/discussions/683)
- [Benchmark graphs for testing community detection (Lancichinetti et al., arxiv 0805.4770)](https://arxiv.org/abs/0805.4770)
- [Comparative analysis of community detection on artificial networks (PMC)](https://pmc.ncbi.nlm.nih.gov/articles/PMC4967864/)
- [AWS Neptune Analytics — Louvain algorithm docs](https://docs.aws.amazon.com/neptune-analytics/latest/userguide/louvain.html)
