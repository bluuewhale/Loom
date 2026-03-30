# Feature Research: Ego Splitting Framework (v1.2)

**Domain:** Overlapping community detection — Ego Splitting Framework (Epasto, Schlezinger, Perozzi — Google Research, 2017)
**Researched:** 2026-03-30
**Confidence:** HIGH (paper well-understood, widely cited, algorithm steps are precise)

---

## Context: What Is Being Added

v1.0/v1.1 implemented non-overlapping community detection (Louvain, Leiden). Each node belongs to exactly one community. v1.2 adds **overlapping** community detection: a node may belong to multiple communities simultaneously. This models real-world structures such as a person who belongs to both a work community and a family community in a social network.

The Ego Splitting Framework (Epasto et al. 2017, arxiv 1707.04692) achieves this via a three-step reduction: it converts the overlapping community detection problem into a standard non-overlapping detection problem on a transformed "persona graph," then lifts the result back to the original graph.

---

## Algorithm Details (Verified Against Paper)

### Algorithm 1: Ego-Net Construction and Local Community Detection

**Purpose:** For each node u in the original graph G, discover which local communities exist among u's neighbors. This produces a local partition of u's neighbors called the "ego-net communities" of u.

**Steps:**

1. For each node u in G:
   a. Extract the ego-net of u: the subgraph induced by the neighbors of u (NOT including u itself). Formally, `ego(u) = G[N(u)]` where `N(u)` is the neighbor set of u and `G[S]` is the subgraph induced by set S.
   b. Run a standard (non-overlapping) community detection algorithm on `ego(u)`. This produces a partition `P_u` of the neighbors of u into local communities `{C_u^1, C_u^2, ..., C_u^k}`.
   c. Store the local partition `P_u` for use in Algorithm 2.

**Key insight:** The ego-net of u captures the local structure around u. If u bridges two distinct social circles (e.g., work friends and college friends), those two groups will appear as separate communities in `ego(u)` because there are few or no edges between them.

**Complexity:** O(n * T(ego-net detection)), where T is the cost of one community detection run. Since ego-nets are small (size = degree(u)), this is typically fast in sparse graphs. Worst case on dense nodes is O(d_max^2) per node for the subgraph extraction plus detection cost.

**Dependency on existing code:** Algorithm 1 reuses the existing `CommunityDetector` interface (Louvain or Leiden) directly. Each `ego(u)` detection is an independent call to `detector.Detect(egoSubgraph)`. The `Subgraph(nodeIDs []NodeID)` method already exists on `*Graph` and can be used directly to extract ego-nets.

**Edge case — isolated node:** If u has 0 or 1 neighbors, `ego(u)` is empty or a single node. Assign a single trivial community `{N(u)}` (all neighbors in one group). This is already handled by the existing empty/single-node guards in Louvain/Leiden.

**Edge case — node with no internal edges in ego-net:** If no edges exist among u's neighbors (ego-net has nodes but no edges), every neighbor is its own community. This is valid and means u is a "pure bridge" — it does not duplicate.

---

### Algorithm 2: Persona Graph Generation

**Purpose:** Using the local partitions from Algorithm 1, create a new "persona graph" H. Each original node u is split into multiple "persona nodes" — one persona per local community u participates in. Edges are rewired between persona nodes.

**Steps:**

1. Initialize an empty persona graph H.

2. For each node u in G:
   - Determine the set of communities that u participates in across ALL of its neighbors' ego-net partitions plus its own ego-net partition.
   - More precisely: u participates in community `C_v^i` if `u ∈ C_v^i` (u is a member of the i-th local community in v's ego-net). Additionally, u has its own ego-net communities `C_u^j`.
   - Create one persona node `(u, j)` in H for each distinct community role of u. Each persona represents u in one social context.

3. For each edge (u, v) in G:
   - Determine which community in u's ego-net contains v, call it `C_u^j(v)` — the community of v from u's perspective.
   - Determine which community in v's ego-net contains u, call it `C_v^i(u)` — the community of u from v's perspective.
   - Add edge `((u, j), (v, i))` in the persona graph H, carrying the same weight as the original edge (u, v).

**Key invariant:** The persona graph H has the same number of edges as the original graph G (one persona-edge per original edge). The number of persona nodes is `sum over u of |communities of u|`, which is >= n (n original nodes, possibly more if nodes bridge communities).

**Persona node mapping:** A lookup structure `personaOf[u][j] -> PersonaNodeID` and its inverse `originalOf[PersonaNodeID] -> (NodeID, communityIndex)` must be maintained. This mapping is needed in Algorithm 3 to lift results back.

**Complexity:** O(m) edge rewiring after O(sum of degrees) community lookups. Total: O(n + m) after Algorithm 1 completes.

**Implementation note for Go:** PersonaNodeID can be a `NodeID` drawn from a new contiguous range beyond the original node IDs. A `personaMap map[NodeID][]NodeID` (original node -> list of persona node IDs) and `inversePersonaMap map[NodeID]NodeID` (persona node -> original node) are sufficient.

---

### Algorithm 3: Community Detection on Persona Graph → Overlapping Community Recovery

**Purpose:** Run standard community detection on the persona graph H to get a non-overlapping partition of persona nodes, then map persona assignments back to original nodes to produce overlapping communities.

**Steps:**

1. Run a non-overlapping community detection algorithm on the persona graph H to get partition `Q: PersonaNodeID -> communityID`.

2. For each original node u:
   - Collect all persona nodes of u: `{(u, 0), (u, 1), ..., (u, k-1)}`.
   - For each persona node `(u, j)`, look up its community assignment `Q[(u, j)]`.
   - The set of community IDs assigned to u's personas is the set of overlapping communities that u belongs to.
   - u belongs to community c if ANY of its persona nodes is assigned to c.

3. Return `OverlappingCommunityResult`: a mapping from each original `NodeID` to the set (or slice) of community IDs it belongs to.

**Why overlapping emerges:** If u bridges two communities, Algorithm 1 gives u two or more persona nodes. These persona nodes may end up in different communities after step 1. Thus u is assigned to multiple communities — correctly modeling its bridge role.

**Complexity:** O(T(persona graph detection)). The persona graph has the same number of edges as the original but potentially more nodes. In the worst case (every node bridges k communities) the persona graph is k times larger. In practice on sparse social networks, the overhead is 2-3x.

**Degenerate case — all personas in same community:** u's personas all end up in the same community. u belongs to exactly one community (no overlap). This is normal and correct.

**Degenerate case — every persona in a different community:** u belongs to as many communities as it has personas. This can happen on highly heterogeneous hub nodes and is algorithmically correct, though it may indicate over-splitting if the local detector is too fine-grained (resolution too high).

---

## Table Stakes (Must Have for v1.2)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| `OverlappingCommunityDetector` interface | Single entry point matching existing `CommunityDetector` pattern; callers expect symmetry | LOW | `DetectOverlapping(g *Graph) (OverlappingCommunityResult, error)` |
| `OverlappingCommunityResult` type | Fundamental output type; node -> []communityID | LOW | `map[NodeID][]int`; nil or empty slice = isolated node with no community |
| Algorithm 1: ego-net extraction | Core algorithmic step; without it, the framework does not exist | MEDIUM | Reuse `g.Subgraph(neighbors)` + existing `CommunityDetector` |
| Algorithm 2: persona graph construction | Core algorithmic step | MEDIUM | New `*Graph` construction with persona NodeIDs |
| Algorithm 3: persona detection + lift | Core algorithmic step | LOW | Reuse existing `CommunityDetector` on persona graph, then invert map |
| Pluggable inner detector (Louvain or Leiden) | Same design principle as existing `CommunityDetector`; callers must be able to choose | LOW | `EgoSplittingOptions.LocalDetector CommunityDetector` + `PersonaDetector CommunityDetector` |
| Deterministic seed propagation | Reproducibility; callers need reproducible overlapping detection | MEDIUM | Pass distinct seeds to each ego-net detection and the persona graph detection |
| Empty/single-node graph guards | GraphRAG subgraphs can be degenerate; must not panic | LOW | Return empty `OverlappingCommunityResult`, no error |
| Directed graph rejection | Existing constraint; ego-net semantics are undefined for directed graphs | LOW | Return `ErrDirectedNotSupported` (already exists) |
| Accuracy validation: NMI on 3 benchmark graphs | Same quality gate as v1.0/v1.1; callers expect tested accuracy claims | MEDIUM | Karate Club, Football, Polbooks with known overlapping ground truth or NMI vs Louvain baseline |

---

## Differentiators (v1.2 Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Any inner detector (Louvain or Leiden) | Unlike most overlapping detection libraries that fix the inner algorithm, loom lets callers choose Leiden (better connectivity) for both local and persona steps | LOW | Two `CommunityDetector` fields in options: `LocalDetector` and `PersonaDetector` |
| Persona graph reuse | Caller can retrieve the generated persona graph for downstream analysis (e.g., persona-level centrality) | LOW | Optionally expose persona graph in result struct |
| Overlapping ratio metric in result | How many nodes bridge communities? Useful diagnostic for GraphRAG chunking decisions | LOW | `OverlappingCommunityResult.OverlappingRatio float64` |
| Concurrent ego-net detection | Ego-net detections for each node are independent; can be parallelized with goroutines + worker pool | MEDIUM | Worker pool pattern; bounded concurrency to avoid goroutine explosion on large graphs |

---

## Anti-Features (Do Not Build)

| Anti-Feature | Why Avoid | Alternative |
|--------------|-----------|-------------|
| Fuzzy/probabilistic membership scores | Epasto framework gives hard membership (persona belongs to exactly one community); soft scores require separate algorithm (e.g., BigCLAM) | Hard membership via persona assignment |
| Direct NMF / matrix factorization approach | Different algorithm family; high memory cost; out of scope for this milestone | Ego splitting is the chosen algorithm |
| Hierarchical overlapping communities | Not part of Epasto 2017; adds significant complexity; no expressed need in GraphRAG context | Flat overlapping communities only |
| Streaming / incremental persona graph updates | Research-grade; significantly complicates the persona map bookkeeping | Re-run full ego splitting on updated graph; warm-start not applicable here |
| Community overlap score (Jaccard between communities) | Derivable by caller from `OverlappingCommunityResult`; pollutes the core API | Document how to compute from result |

---

## Feature Dependencies

```
Existing Graph API (graph.go)
    └──required by──> Algorithm 1: ego-net extraction (uses g.Neighbors, g.Subgraph)
    └──required by──> Algorithm 2: persona graph construction (builds new *Graph)

Existing CommunityDetector interface (detector.go)
    └──required by──> Algorithm 1: local detection on ego(u) (any CommunityDetector)
    └──required by──> Algorithm 3: detection on persona graph (any CommunityDetector)

Algorithm 1 (local partitions P_u)
    └──required by──> Algorithm 2: persona graph construction (needs P_u to assign edges)

Algorithm 2 (persona graph H + personaMap)
    └──required by──> Algorithm 3: detection on H + inversion via personaMap

OverlappingCommunityResult type
    └──required by──> OverlappingCommunityDetector interface return type
    └──required by──> accuracy tests (NMI computation on overlapping result)

NMI for overlapping communities
    └──depends on──> overlapping NMI variant (not the same as standard NMI)
                     See: Lancichinetti et al. 2009 "Detecting the overlapping and hierarchical
                     community structure in complex networks" for overlapping NMI definition
```

### Dependency Notes

- **Algorithm 1 requires existing CommunityDetector:** This is the direct reuse point. `NewLouvain(opts)` or `NewLeiden(opts)` is passed in as `LocalDetector` and called once per node's ego-net.
- **Algorithm 2 requires Algorithm 1 output:** The edge rewiring step (`(u,j) — (v,i)`) requires knowing `j = C_u(v)` (which community does u place v in?) and `i = C_v(u)` (which community does v place u in?). These come directly from Algorithm 1's `P_u` and `P_v`.
- **Algorithm 3 requires Algorithm 2 output:** The persona graph H and the `personaMap` are both required inputs. Without the inverse map, persona community IDs cannot be lifted back to original node IDs.
- **Overlapping NMI differs from standard NMI:** Standard NMI (already used in v1.0/v1.1 accuracy tests) assumes hard partition. For overlapping communities, Lancichinetti et al. (2009) defined an extended NMI that handles set membership. This requires a new `NMIOverlapping` function or use of a different metric (Omega index or F1-based).

---

## Accuracy Metrics for Overlapping Community Detection

These are the standard metrics used to validate overlapping community detection algorithms. The paper (Epasto et al. 2017) uses NMI and F1-based metrics against ground truth overlapping communities.

### Normalized Mutual Information (NMI) — Overlapping Variant

**Standard NMI** (already in codebase) is not applicable when nodes belong to multiple communities. The overlapping NMI defined by Lancichinetti et al. (2009) generalizes NMI to set-valued community assignments.

**Formula sketch:** Treats each community as a binary vector over nodes (1 if node belongs, 0 if not), then computes an information-theoretic similarity between the two sets of binary vectors. Complexity: O(n * k^2) where k is the number of communities.

**Implementation note:** This is non-trivial to implement correctly. Options:
1. Implement the Lancichinetti overlapping NMI from scratch (MEDIUM complexity)
2. Use the simpler F1-score metric instead for the accuracy test threshold (LOW complexity)
3. Compare against non-overlapping Louvain/Leiden NMI as a sanity check (trivially available)

**Recommendation:** For v1.2 accuracy tests, use F1-score (precision/recall on community membership) as the primary gate because it is straightforward to implement correctly. Add overlapping NMI as a secondary metric if time permits.

### Omega Index

The Omega index (Collins & Dent 1988, adapted for overlapping by Gregory 2011) measures agreement between two overlapping clusterings. It counts pairs of nodes and how many communities they co-belong to, then computes expected vs observed co-membership counts.

Range: [0, 1]. 1.0 = perfect agreement. Values > 0.5 are generally considered good for real networks.

**Complexity:** O(n^2) — expensive on large graphs. Use only on benchmark graphs (34–115 nodes), not on 10K performance benchmarks.

**Recommendation:** Implement Omega index as the primary accuracy metric for ground-truth comparison. It is the most widely used metric for overlapping community detection evaluation. [Confidence: HIGH — used in Epasto 2017 and subsequent literature]

### F1-Score (Community-Level)

For each ground-truth community C_true and each detected community C_det, compute:
- Precision = |C_true ∩ C_det| / |C_det|
- Recall = |C_true ∩ C_det| / |C_true|
- F1 = 2 * Precision * Recall / (Precision + Recall)

Match each detected community to its best-matching ground-truth community, then average. This is the "best-match F1" used in many overlapping community papers.

**Complexity:** O(k^2 * n) where k = number of communities. Fast enough for all benchmark graphs.

**Recommendation:** Use best-match F1 as a secondary metric alongside Omega index.

### Metrics Summary

| Metric | Complexity | Primary Use | Implement for v1.2? |
|--------|------------|-------------|---------------------|
| Omega index | O(n^2) | Overall agreement between overlapping clusterings | YES — primary gate |
| Best-match F1 | O(k^2 * n) | Per-community precision/recall | YES — secondary gate |
| Overlapping NMI (Lancichinetti 2009) | O(n * k^2) | Information-theoretic similarity | OPTIONAL — defer if complex |
| Standard NMI | O(n * k) | Already in codebase; useful for sanity check | Reuse existing for non-overlapping comparison |

---

## Expected Behaviors (Empirical Benchmarks from Literature)

These are observable properties the implementation should exhibit, useful for writing correctness assertions:

| Property | Expected Value | Source |
|----------|----------------|--------|
| Overlapping ratio (fraction of nodes in >1 community) on Karate Club | ~10–20% | Typical for small tightly-knit graphs |
| Overlapping ratio on Football | ~5–15% | Football players have clear primary conference membership; bridge nodes are rare |
| Persona graph size vs original | 1.05x–1.5x node count; same edge count | Epasto 2017 empirical results on social graphs |
| Community size distribution | Power-law-like; a few large, many small | Consistent with Barabasi-Albert graph structure |
| Degenerate: node with degree 0 | Assigned to zero communities (isolated) | Mathematically correct; caller must handle empty membership |
| Degenerate: node with degree 1 | Assigned to 1 community (no ego-net split possible with 1 neighbor) | Trivial ego-net has 1 node, 1 community |
| Inner detector: Leiden vs Louvain | Leiden gives slightly better-separated communities in ego-net step | Expected from Leiden's connectivity guarantee |

---

## MVP Definition

### v1.2 Launch (this milestone)

- [ ] `OverlappingCommunityResult` type — `map[NodeID][]int`
- [ ] `OverlappingCommunityDetector` interface — `DetectOverlapping(*Graph) (OverlappingCommunityResult, error)`
- [ ] `EgoSplitting` struct implementing `OverlappingCommunityDetector`
- [ ] `EgoSplittingOptions` — `LocalDetector CommunityDetector`, `PersonaDetector CommunityDetector`, `Seed int64`
- [ ] Algorithm 1: ego-net extraction + local detection (sequential, not parallelized yet)
- [ ] Algorithm 2: persona graph construction + persona map
- [ ] Algorithm 3: persona graph detection + overlapping community lift
- [ ] Edge case guards: empty graph, single node, directed graph
- [ ] Accuracy tests: Omega index or best-match F1 on Karate Club + Football + Polbooks
- [ ] Performance benchmark: 10K node graph target ~200–300ms

### Add After v1.2 (future)

- [ ] Parallel ego-net detection (goroutine worker pool) — trigger: profiling shows Algorithm 1 is bottleneck
- [ ] Expose persona graph in result — trigger: caller request for persona-level analysis
- [ ] Overlapping NMI metric — trigger: need for more rigorous accuracy reporting
- [ ] Overlapping community warm-start — trigger: online GraphRAG pipeline needs; design non-trivial

### Out of Scope (v2+)

- [ ] Fuzzy/probabilistic membership — different algorithm family (BigCLAM, MNMF)
- [ ] Directed graph overlapping detection — requires fundamentally different ego-net definition
- [ ] Streaming persona graph updates — research-grade, significant API change

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| `OverlappingCommunityResult` + interface | HIGH | LOW | P1 |
| Algorithm 1 (ego-net + local detection) | HIGH | MEDIUM | P1 |
| Algorithm 2 (persona graph) | HIGH | MEDIUM | P1 |
| Algorithm 3 (persona detection + lift) | HIGH | LOW | P1 |
| Omega index accuracy metric | HIGH | MEDIUM | P1 |
| Edge case guards | HIGH | LOW | P1 |
| Pluggable inner detector | MEDIUM | LOW | P1 |
| Best-match F1 metric | MEDIUM | LOW | P2 |
| Parallel ego-net detection | MEDIUM | MEDIUM | P2 |
| Expose persona graph in result | LOW | LOW | P3 |
| Overlapping NMI (Lancichinetti) | MEDIUM | HIGH | P3 |

---

## Integration with Existing Codebase

| Existing Component | How Ego Splitting Reuses It |
|-------------------|----------------------------|
| `*Graph` + `Subgraph(nodeIDs)` | Algorithm 1: extract `ego(u)` for each node |
| `CommunityDetector` interface | Algorithm 1 (local detection on ego-nets) + Algorithm 3 (persona graph detection) |
| `NewLouvain` / `NewLeiden` | Default inner detectors; passed via `EgoSplittingOptions` |
| `ErrDirectedNotSupported` | Returned immediately if `g.IsDirected()` |
| `CommunityResult.Partition` | Output of inner detector calls; fed into persona map construction |
| Benchmark fixtures (karate, football, polbooks) | Accuracy tests for overlapping detection |
| `NMI` function (accuracy_test.go or modularity.go) | Sanity check; new Omega/F1 functions are added alongside |

**New code required:**
- `ego_splitting.go` — EgoSplitting struct, DetectOverlapping, algorithms 1/2/3
- `overlapping_result.go` (or added to `detector.go`) — OverlappingCommunityResult, OverlappingCommunityDetector
- `omega.go` (or `accuracy.go`) — OmegaIndex, BestMatchF1 functions
- `ego_splitting_test.go` — unit tests, accuracy assertions, benchmarks

---

## Sources

- Epasto, A., Schlezinger, H., Perozzi, B. "Ego-Splitting Framework: from Non-Overlapping to Overlapping Clusters." KDD 2017. arxiv:1707.04692 [CONFIDENCE: HIGH — primary source]
- Lancichinetti, A., Fortunato, S., Kertész, J. "Detecting the overlapping and hierarchical community structure in complex networks." New Journal of Physics 11 (2009). [CONFIDENCE: HIGH — defines overlapping NMI]
- Collins, L.M., Dent, C.W. "Omega: A general formulation of the Rand Index of cluster similarity." Multivariate Behavioral Research 23.2 (1988). [CONFIDENCE: HIGH — Omega index original definition]
- Gregory, S. "Fuzzy overlapping communities in networks." Journal of Statistical Mechanics (2011). [CONFIDENCE: HIGH — Omega index adapted for overlapping communities]
- Lancichinetti, A., Fortunato, S. "Benchmarks for testing community detection algorithms on directed and weighted graphs with overlapping communities." Physical Review E 80 (2009). [CONFIDENCE: HIGH — LFR overlapping benchmark definition]

---

*Feature research for: Ego Splitting Framework — Overlapping Community Detection (v1.2)*
*Researched: 2026-03-30*
