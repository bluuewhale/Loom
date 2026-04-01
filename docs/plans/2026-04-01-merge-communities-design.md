# Design: Merge Small Communities

**Date**: 2026-04-01  
**Branch**: feat/merge-communities  
**Status**: Approved

---

## Problem

After community detection (Louvain / Leiden / EgoSplitting), graphs with STAR
topology frequently produce many tiny communities — isolated leaf nodes or
small spoke clusters that have no semantic value in a GraphRAG pipeline.
There is currently no post-processing utility to consolidate these fragments.

---

## Goals

- Post-process a detected partition to absorb communities below a size threshold
  into their most appropriate neighbour.
- Support both **disjoint** (`CommunityResult`) and **overlapping**
  (`OverlappingCommunityResult`) partitions.
- Expose a threshold that can be expressed as an **absolute node count**
  (`MinSize`) and/or a **relative fraction** of total nodes (`MinFraction`);
  either condition triggers a merge.
- Support two merge-target strategies: **connectivity** (fast, default) and
  **modularity gain** (more principled, slightly slower).
- Fit the existing `LouvainOptions` / `LeidenOptions` API style: options struct
  + top-level function, zero value = valid no-op.

---

## Public API

```go
// MergeStrategy controls how a small community selects its merge target.
type MergeStrategy int

const (
    // MergeByConnectivity merges into the neighbouring community with the
    // highest total edge weight. O(edges in small community) per decision.
    MergeByConnectivity MergeStrategy = iota

    // MergeByModularity merges into the neighbour that yields the greatest
    // modularity gain (or least loss). O(k) factor vs Connectivity.
    MergeByModularity
)

// MergeOptions configures small-community merging.
// Zero value is valid: MinSize=0 and MinFraction=0.0 → no merging performed.
type MergeOptions struct {
    // MinSize: communities with fewer than MinSize nodes are merge candidates.
    MinSize int

    // MinFraction: communities smaller than MinFraction * totalNodes are
    // merge candidates. Useful when graph sizes vary across pipeline runs.
    MinFraction float64

    // Strategy selects the merge-target rule. Default: MergeByConnectivity.
    Strategy MergeStrategy

    // Resolution scales the modularity penalty term (MergeByModularity only).
    // Default 1.0. Higher values favour more / smaller communities.
    Resolution float64
}

// MergeSmallCommunities post-processes a disjoint partition by absorbing
// communities that satisfy the MinSize or MinFraction threshold into their
// best neighbour according to Strategy. Community IDs in the returned result
// are re-numbered to be contiguous. Modularity is recomputed on the merged
// partition.
func MergeSmallCommunities(
    g *Graph,
    result CommunityResult,
    opts MergeOptions,
) (CommunityResult, error)

// MergeSmallOverlappingCommunities does the same for overlapping partitions.
// Community size is measured by the number of unique member nodes.
// Merging two overlapping communities unions their node memberships and
// updates NodeCommunities accordingly.
func MergeSmallOverlappingCommunities(
    g *Graph,
    result OverlappingCommunityResult,
    opts MergeOptions,
) (OverlappingCommunityResult, error)
```

---

## Algorithms

### Disjoint — `MergeSmallCommunities`

```
1. Compute communitySize[c] for all communities.
2. threshold = max(MinSize, ceil(MinFraction * totalNodes))
3. candidates = {c : communitySize[c] < threshold}, sorted ascending by size.
4. For each candidate c (smallest first):
     neighbors = set of communities connected to c by at least one edge
     if neighbors == ∅: skip (isolated community — leave in place)
     target =
       MergeByConnectivity → argmax_{n ∈ neighbors} WeightToComm(c, n)
       MergeByModularity   → argmax_{n ∈ neighbors} ΔQ(c → n)
     Reassign all nodes in c to target.
     Update communitySize[target] += communitySize[c].
     If communitySize[target] ≥ threshold: remove target from candidates.
5. Re-number community IDs to 0-indexed contiguous.
6. Return CommunityResult{Partition, Modularity: ComputeModularity(g, partition)}.
```

**ΔQ formula** (MergeByModularity):
```
ΔQ(c → t) = 2·w(c,t)/m  −  2·γ·s(c)·s(t)/m²

  w(c,t) = total edge weight between communities c and t  (via WeightToComm)
  s(c)   = total strength of community c (internal + external edges)
  m      = total graph edge weight
  γ      = Resolution (default 1.0)
```

### Overlapping — `MergeSmallOverlappingCommunities`

Community size = number of unique member nodes in `Communities[i]`.

```
1. candidates = {i : len(Communities[i]) < threshold}, ascending by size.
2. For each candidate i:
     neighbors = all communities that share at least one node with Communities[i]
                 (via NodeCommunities lookup)
     target = neighbor with maximum |Communities[i] ∩ Communities[neighbor]|
              (tie-break: Strategy — connectivity weight sum or ΔQ)
     Communities[target] = union(Communities[target], Communities[i])
     Update NodeCommunities for newly added nodes.
     Mark Communities[i] as removed.
3. Compact Communities slice (remove nils), re-index NodeCommunities.
```

---

## Error Handling

```go
// Returned when partition references node IDs absent from the graph.
var ErrPartitionGraphMismatch = errors.New("partition contains node IDs not present in graph")

// Returned for invalid option values (negative MinSize, MinFraction outside [0,1]).
var ErrInvalidMergeOptions = errors.New("invalid merge options")
```

Silent no-ops (return input unchanged, no error):
- `MinSize == 0 && MinFraction == 0.0`
- No communities satisfy the threshold
- A small community has no neighbours (isolated) — skipped in place

---

## Test Cases

| Case | Purpose |
|------|---------|
| 3-node star (hub + 2 leaves, each leaf its own community) | Core merge behaviour |
| All communities already above threshold | No-op verification |
| Isolated small community (no edges out) | Skip-in-place behaviour |
| MinFraction-only trigger on large graph | Relative threshold verification |
| MergeByModularity vs MergeByConnectivity on same input | Strategy difference |
| Overlapping: two small communities unioned | Overlapping merge + NodeCommunities consistency |
| Invalid options (negative MinSize) | Error path |

---

## Non-Goals

- Splitting communities (separate concern).
- Online / incremental merge after `Update()` calls (can be added later).
- Overlapping-community modularity recomputation (no accepted formula; omitted from returned result).
