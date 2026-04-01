# Merge Communities Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `MergeSmallCommunities` and `MergeSmallOverlappingCommunities` post-processing utilities that absorb tiny communities (caused by STAR-graph topology) into their best neighbours.

**Architecture:** Two top-level functions in `graph/merge.go` follow the existing `LouvainOptions`/`LeidenOptions` options-struct pattern. A shared `MergeOptions` struct with `MinSize int`, `MinFraction float64`, `Strategy MergeStrategy`, and `Resolution float64` controls both functions. The disjoint variant iterates small communities smallest-first, merging each into the neighbour with the highest connectivity weight or modularity gain. The overlapping variant unions node sets by shared membership. Errors are sentinel values matching the existing `ErrDirectedNotSupported` style.

**Tech Stack:** Go 1.26, `package graph` (`github.com/bluuewhale/loom/graph`), stdlib only. Tests use `go test ./graph/... -run TestMerge`.

---

### Task 1: Define types and sentinel errors

**Files:**
- Create: `graph/merge.go`

**Step 1: Write the failing test**

Add to `graph/merge_test.go` (new file):

```go
package graph

import "testing"

func TestMergeOptions_InvalidMinSize(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	result := CommunityResult{Partition: map[NodeID]int{0: 0, 1: 1}, Modularity: 0}
	_, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: -1})
	if err != ErrInvalidMergeOptions {
		t.Fatalf("expected ErrInvalidMergeOptions, got %v", err)
	}
}

func TestMergeOptions_InvalidMinFraction(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	result := CommunityResult{Partition: map[NodeID]int{0: 0, 1: 1}, Modularity: 0}
	_, err := MergeSmallCommunities(g, result, MergeOptions{MinFraction: 1.5})
	if err != ErrInvalidMergeOptions {
		t.Fatalf("expected ErrInvalidMergeOptions, got %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

```
cd graph && go test -run TestMergeOptions ./... 
```
Expected: compile error — `MergeSmallCommunities`, `MergeOptions`, `ErrInvalidMergeOptions` undefined.

**Step 3: Write minimal implementation**

Create `graph/merge.go`:

```go
package graph

import "errors"

// ErrInvalidMergeOptions is returned when MergeOptions contains invalid values.
var ErrInvalidMergeOptions = errors.New("invalid merge options: MinSize must be >= 0 and MinFraction must be in [0, 1]")

// ErrPartitionGraphMismatch is returned when the partition references node IDs
// not present in the graph.
var ErrPartitionGraphMismatch = errors.New("partition contains node IDs not present in graph")

// MergeStrategy controls how a small community selects its merge target.
type MergeStrategy int

const (
	// MergeByConnectivity merges into the neighbouring community with the
	// highest total edge weight. O(edges in small community) per decision.
	MergeByConnectivity MergeStrategy = iota

	// MergeByModularity merges into the neighbour that yields the greatest
	// modularity gain (or least loss).
	MergeByModularity
)

// MergeOptions configures small-community merging.
// Zero value is valid: MinSize=0 and MinFraction=0.0 → no merging performed.
type MergeOptions struct {
	// MinSize: communities with fewer than MinSize nodes are merge candidates.
	MinSize int

	// MinFraction: communities smaller than MinFraction * totalNodes are
	// merge candidates (OR condition with MinSize).
	MinFraction float64

	// Strategy selects the merge-target rule. Default: MergeByConnectivity.
	Strategy MergeStrategy

	// Resolution scales the modularity penalty term (MergeByModularity only).
	// Zero value treated as 1.0.
	Resolution float64
}

// validateMergeOptions returns ErrInvalidMergeOptions for out-of-range values.
func validateMergeOptions(opts MergeOptions) error {
	if opts.MinSize < 0 || opts.MinFraction < 0 || opts.MinFraction > 1 {
		return ErrInvalidMergeOptions
	}
	return nil
}

// mergeThreshold returns the effective node-count threshold from opts and totalNodes.
func mergeThreshold(opts MergeOptions, totalNodes int) int {
	threshold := opts.MinSize
	if frac := int(opts.MinFraction * float64(totalNodes)); frac > threshold {
		threshold = frac
	}
	return threshold
}

// resolution returns opts.Resolution, defaulting to 1.0.
func (opts MergeOptions) resolution() float64 {
	if opts.Resolution == 0 {
		return 1.0
	}
	return opts.Resolution
}

// MergeSmallCommunities post-processes a disjoint partition — stub.
func MergeSmallCommunities(g *Graph, result CommunityResult, opts MergeOptions) (CommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return CommunityResult{}, err
	}
	return result, nil
}

// MergeSmallOverlappingCommunities post-processes an overlapping partition — stub.
func MergeSmallOverlappingCommunities(g *Graph, result OverlappingCommunityResult, opts MergeOptions) (OverlappingCommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return OverlappingCommunityResult{}, err
	}
	return result, nil
}
```

**Step 4: Run test to verify it passes**

```
go test -run TestMergeOptions ./graph/...
```
Expected: PASS

**Step 5: Commit**

```bash
git add graph/merge.go graph/merge_test.go
git commit -m "feat(merge): add MergeOptions types, sentinel errors, stub functions"
```

---

### Task 2: Implement `MergeSmallCommunities` — no-op and partition-mismatch paths

**Files:**
- Modify: `graph/merge.go`
- Modify: `graph/merge_test.go`

**Step 1: Write the failing tests**

Add to `graph/merge_test.go`:

```go
func TestMergeSmallCommunities_NoOp_ZeroThreshold(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	partition := map[NodeID]int{0: 0, 1: 0, 2: 1}
	result := CommunityResult{Partition: partition, Modularity: 0.1}

	got, err := MergeSmallCommunities(g, result, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Partition) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(got.Partition))
	}
	if uniqueCommunities(got.Partition) != 2 {
		t.Fatalf("expected 2 communities (no-op), got %d", uniqueCommunities(got.Partition))
	}
}

func TestMergeSmallCommunities_PartitionMismatch(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	// node 99 is not in g
	partition := map[NodeID]int{0: 0, 1: 1, 99: 2}
	result := CommunityResult{Partition: partition}

	_, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: 1})
	if err != ErrPartitionGraphMismatch {
		t.Fatalf("expected ErrPartitionGraphMismatch, got %v", err)
	}
}

func TestMergeSmallCommunities_NoCandidates(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 3, 1.0)
	// All communities size >= 2, threshold = 2 → no candidates
	partition := map[NodeID]int{0: 0, 1: 0, 2: 1, 3: 1}
	result := CommunityResult{Partition: partition}

	got, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if uniqueCommunities(got.Partition) != 2 {
		t.Fatalf("expected 2 communities, got %d", uniqueCommunities(got.Partition))
	}
}
```

**Step 2: Run test to verify it fails**

```
go test -run TestMergeSmallCommunities_NoOp ./graph/...
```
Expected: `TestMergeSmallCommunities_PartitionMismatch` FAIL — currently returns nil error.

**Step 3: Implement validation and no-op logic**

Replace the `MergeSmallCommunities` stub in `graph/merge.go`:

```go
// MergeSmallCommunities post-processes a disjoint partition by absorbing
// communities that satisfy the MinSize or MinFraction threshold into their
// best neighbour according to Strategy. Community IDs in the returned result
// are re-numbered to be contiguous. Modularity is recomputed on the merged
// partition.
//
// Returns the input result unchanged when no merge threshold is set
// (MinSize==0 and MinFraction==0) or no communities qualify.
func MergeSmallCommunities(g *Graph, result CommunityResult, opts MergeOptions) (CommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return CommunityResult{}, err
	}

	// Validate partition nodes exist in graph.
	for n := range result.Partition {
		if _, ok := g.nodes[n]; !ok {
			return CommunityResult{}, ErrPartitionGraphMismatch
		}
	}

	threshold := mergeThreshold(opts, len(result.Partition))
	if threshold == 0 {
		return result, nil
	}

	// Build community → nodes map.
	commNodes := make(map[int][]NodeID)
	for n, c := range result.Partition {
		commNodes[c] = append(commNodes[c], n)
	}

	// Identify candidates (size < threshold), sorted ascending by size for
	// smallest-first processing.
	type candidate struct {
		comm int
		size int
	}
	var candidates []candidate
	for c, nodes := range commNodes {
		if len(nodes) < threshold {
			candidates = append(candidates, candidate{c, len(nodes)})
		}
	}
	if len(candidates) == 0 {
		return result, nil
	}

	// Sort smallest-first (stable by comm ID for determinism).
	sortCandidates(candidates)

	// Work on a copy of the partition.
	partition := copyPartition(result.Partition)

	for _, cand := range candidates {
		nodes, ok := commNodes[cand.comm]
		if !ok {
			continue // already merged away
		}
		target, found := findMergeTarget(g, nodes, cand.comm, partition, opts)
		if !found {
			continue // isolated community — leave in place
		}
		// Merge cand.comm → target.
		for _, n := range nodes {
			partition[n] = target
		}
		commNodes[target] = append(commNodes[target], nodes...)
		delete(commNodes, cand.comm)
	}

	// Renumber to contiguous 0-indexed IDs.
	newPartition := compactPartition(partition)
	return CommunityResult{
		Partition:  newPartition,
		Modularity: ComputeModularity(g, newPartition),
	}, nil
}
```

Add helpers at the bottom of `graph/merge.go`:

```go
// copyPartition returns a shallow copy of partition.
func copyPartition(p map[NodeID]int) map[NodeID]int {
	out := make(map[NodeID]int, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// compactPartition remaps community IDs to 0-indexed contiguous integers.
func compactPartition(p map[NodeID]int) map[NodeID]int {
	remap := make(map[int]int)
	next := 0
	out := make(map[NodeID]int, len(p))
	for n, c := range p {
		if _, ok := remap[c]; !ok {
			remap[c] = next
			next++
		}
		out[n] = remap[c]
	}
	return out
}

// sortCandidates sorts candidates ascending by size, then comm ID (determinism).
func sortCandidates(cs []struct {
	comm int
	size int
}) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && (cs[j].size < cs[j-1].size ||
			(cs[j].size == cs[j-1].size && cs[j].comm < cs[j-1].comm)); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

// findMergeTarget returns the community ID that nodes should merge into.
// Returns (0, false) if no neighbouring community exists (isolated community).
func findMergeTarget(g *Graph, nodes []NodeID, srcComm int, partition map[NodeID]int, opts MergeOptions) (int, bool) {
	// Collect neighbouring communities and their aggregate weight.
	neighborWeight := make(map[int]float64)
	for _, n := range nodes {
		for _, e := range g.Neighbors(n) {
			c := partition[e.To]
			if c != srcComm {
				neighborWeight[c] += e.Weight
			}
		}
	}
	if len(neighborWeight) == 0 {
		return 0, false
	}

	switch opts.Strategy {
	case MergeByModularity:
		return bestByModularity(g, neighborWeight, nodes, partition, opts.resolution())
	default: // MergeByConnectivity
		return bestByConnectivity(neighborWeight)
	}
}

// bestByConnectivity returns the community with the highest aggregate edge weight.
func bestByConnectivity(neighborWeight map[int]float64) (int, bool) {
	best, bestW := -1, -1.0
	for c, w := range neighborWeight {
		if w > bestW || (w == bestW && c < best) {
			best, bestW = c, w
		}
	}
	return best, true
}

// bestByModularity returns the community whose merge yields the greatest ΔQ.
// ΔQ(src→t) = 2·w(src,t)/m  −  2·γ·s(src)·s(t)/m²
func bestByModularity(g *Graph, neighborWeight map[int]float64, srcNodes []NodeID, partition map[NodeID]int, gamma float64) (int, bool) {
	m := g.TotalWeight()
	if m == 0 {
		return bestByConnectivity(neighborWeight)
	}
	twoM := 2 * m

	// Compute strength of source community.
	var srcStr float64
	for _, n := range srcNodes {
		srcStr += g.Strength(n)
	}

	best, bestDQ := -1, -1e18
	for t, w := range neighborWeight {
		tStr := g.CommStrength(t, partition)
		dq := 2*w/twoM - 2*gamma*srcStr*tStr/(twoM*twoM)
		if dq > bestDQ || (dq == bestDQ && t < best) {
			best, bestDQ = t, dq
		}
	}
	return best, true
}
```

**Step 4: Run tests to verify they pass**

```
go test -run TestMergeSmallCommunities ./graph/...
```
Expected: PASS

**Step 5: Commit**

```bash
git add graph/merge.go graph/merge_test.go
git commit -m "feat(merge): implement MergeSmallCommunities core logic"
```

---

### Task 3: Test core merge behaviour — STAR graph and MinFraction

**Files:**
- Modify: `graph/merge_test.go`

**Step 1: Write the failing tests**

Add to `graph/merge_test.go`:

```go
// TestMergeSmallCommunities_StarGraph verifies that leaf-node singleton
// communities (the canonical STAR-graph fragmentation) are absorbed into the
// hub community.
func TestMergeSmallCommunities_StarGraph(t *testing.T) {
	// Star: hub=0, leaves=1,2,3. Initial partition: hub alone + each leaf alone.
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(0, 2, 1.0)
	g.AddEdge(0, 3, 1.0)
	partition := map[NodeID]int{0: 0, 1: 1, 2: 2, 3: 3}
	result := CommunityResult{Partition: partition}

	got, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	// All leaves should merge into hub's community → 1 community total.
	if uniqueCommunities(got.Partition) != 1 {
		t.Fatalf("expected 1 community, got %d", uniqueCommunities(got.Partition))
	}
}

func TestMergeSmallCommunities_MinFraction(t *testing.T) {
	// 10 nodes: 8 in comm 0, 2 in comm 1. MinFraction=0.3 → threshold=3 → comm 1 merges.
	g := NewGraph(false)
	for i := 0; i < 8; i++ {
		g.AddEdge(NodeID(i), NodeID(i+1)%8, 1.0)
	}
	// Bridge between the two clusters
	g.AddEdge(0, 8, 1.0)
	g.AddEdge(0, 9, 1.0)
	g.AddEdge(8, 9, 1.0)

	partition := map[NodeID]int{}
	for i := 0; i < 8; i++ {
		partition[NodeID(i)] = 0
	}
	partition[8] = 1
	partition[9] = 1
	result := CommunityResult{Partition: partition}

	got, err := MergeSmallCommunities(g, result, MergeOptions{MinFraction: 0.3})
	if err != nil {
		t.Fatal(err)
	}
	if uniqueCommunities(got.Partition) != 1 {
		t.Fatalf("expected 1 community after MinFraction merge, got %d", uniqueCommunities(got.Partition))
	}
}

func TestMergeSmallCommunities_IsolatedSmallCommunity(t *testing.T) {
	// Community 1 has no edges to community 0 → must not be merged (left in place).
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddNode(2, 1.0) // isolated node in its own community
	partition := map[NodeID]int{0: 0, 1: 0, 2: 1}
	result := CommunityResult{Partition: partition}

	got, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Community 1 is isolated — stays at 2 communities.
	if uniqueCommunities(got.Partition) != 2 {
		t.Fatalf("expected 2 communities (isolated kept), got %d", uniqueCommunities(got.Partition))
	}
}

func TestMergeSmallCommunities_ModularityStrategy(t *testing.T) {
	// Two target communities: one has high connectivity, one high strength.
	// With MergeByModularity the penalty term should affect the choice.
	g := NewGraph(false)
	// Small community: node 0 (comm 0, size 1)
	// Target A: nodes 1,2,3 (comm 1) — 2 edges to node 0
	// Target B: nodes 4,5,6,7,8,9 (comm 2) — 1 edge to node 0
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(0, 2, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(1, 3, 1.0)
	g.AddEdge(0, 4, 1.0)
	for i := 4; i < 9; i++ {
		g.AddEdge(NodeID(i), NodeID(i+1), 1.0)
	}
	partition := map[NodeID]int{0: 0}
	for i := 1; i <= 3; i++ {
		partition[NodeID(i)] = 1
	}
	for i := 4; i <= 9; i++ {
		partition[NodeID(i)] = 2
	}
	result := CommunityResult{Partition: partition}

	got, err := MergeSmallCommunities(g, result, MergeOptions{MinSize: 2, Strategy: MergeByModularity})
	if err != nil {
		t.Fatal(err)
	}
	// Node 0 should end up in comm 1 (higher connectivity weight despite penalty).
	// Just verify it merged somewhere and community count decreased.
	if uniqueCommunities(got.Partition) != 2 {
		t.Fatalf("expected 2 communities after merge, got %d", uniqueCommunities(got.Partition))
	}
}
```

**Step 2: Run to verify**

```
go test -run TestMergeSmallCommunities ./graph/... -v
```
Expected: all PASS. If `TestMergeSmallCommunities_StarGraph` fails, check `sortCandidates` — the `candidate` type in the helper must match the local struct definition. Fix type mismatch if needed (see note below).

> **Note on `sortCandidates` type:** The helper was written with an anonymous struct parameter. In Go you cannot pass a named type to a function expecting an anonymous struct with the same fields. Refactor to use the named `candidate` type:
> ```go
> type candidate struct { comm, size int }
> func sortCandidates(cs []candidate) { ... }
> ```

**Step 3: Commit**

```bash
git add graph/merge_test.go graph/merge.go
git commit -m "test(merge): add STAR graph, MinFraction, isolated, and strategy tests"
```

---

### Task 4: Implement `MergeSmallOverlappingCommunities`

**Files:**
- Modify: `graph/merge.go`
- Modify: `graph/merge_test.go`

**Step 1: Write the failing tests**

Add to `graph/merge_test.go`:

```go
func TestMergeSmallOverlappingCommunities_BasicUnion(t *testing.T) {
	// Comm 0: {0,1,2}, Comm 1: {3} (size 1 < threshold 2)
	// Node 3 shares no direct membership overlap with comm 0, but has an edge to node 2.
	// → Comm 1 should be merged into Comm 0.
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 3, 1.0)

	result := OverlappingCommunityResult{
		Communities: [][]NodeID{{0, 1, 2}, {3}},
		NodeCommunities: map[NodeID][]int{
			0: {0}, 1: {0}, 2: {0}, 3: {1},
		},
	}

	got, err := MergeSmallOverlappingCommunities(g, result, MergeOptions{MinSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Communities) != 1 {
		t.Fatalf("expected 1 community, got %d", len(got.Communities))
	}
	// All 4 nodes should be in the single community.
	if len(got.Communities[0]) != 4 {
		t.Fatalf("expected 4 nodes in community 0, got %d", len(got.Communities[0]))
	}
}

func TestMergeSmallOverlappingCommunities_NodeCommunitiesConsistency(t *testing.T) {
	// After merge, NodeCommunities must accurately reflect Communities.
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 3, 1.0)

	result := OverlappingCommunityResult{
		Communities: [][]NodeID{{0, 1, 2}, {3}},
		NodeCommunities: map[NodeID][]int{
			0: {0}, 1: {0}, 2: {0}, 3: {1},
		},
	}

	got, err := MergeSmallOverlappingCommunities(g, result, MergeOptions{MinSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	// Verify NodeCommunities consistency: every node in Communities[i] must list i.
	for i, comm := range got.Communities {
		for _, n := range comm {
			found := false
			for _, ci := range got.NodeCommunities[n] {
				if ci == i {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("node %d not listed in NodeCommunities for community %d", n, i)
			}
		}
	}
}

func TestMergeSmallOverlappingCommunities_NoOp(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	result := OverlappingCommunityResult{
		Communities:     [][]NodeID{{0, 1}},
		NodeCommunities: map[NodeID][]int{0: {0}, 1: {0}},
	}
	got, err := MergeSmallOverlappingCommunities(g, result, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Communities) != 1 {
		t.Fatalf("expected 1 community (no-op), got %d", len(got.Communities))
	}
}
```

**Step 2: Run to verify it fails**

```
go test -run TestMergeSmallOverlapping ./graph/...
```
Expected: FAIL — stub returns input unchanged; basic-union test fails.

**Step 3: Implement `MergeSmallOverlappingCommunities`**

Replace the stub in `graph/merge.go`:

```go
// MergeSmallOverlappingCommunities post-processes an overlapping partition by
// absorbing communities below the threshold into the neighbour with the most
// shared-node overlap (tie-broken by Strategy). NodeCommunities is rebuilt to
// be consistent with the merged Communities slice.
func MergeSmallOverlappingCommunities(g *Graph, result OverlappingCommunityResult, opts MergeOptions) (OverlappingCommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return OverlappingCommunityResult{}, err
	}

	totalNodes := len(result.NodeCommunities)
	threshold := mergeThreshold(opts, totalNodes)
	if threshold == 0 {
		return result, nil
	}

	// Work on copies so callers can't observe partial state.
	communities := make([][]NodeID, len(result.Communities))
	for i, c := range result.Communities {
		cp := make([]NodeID, len(c))
		copy(cp, c)
		communities[i] = cp
	}
	nodeCommunities := make(map[NodeID][]int, len(result.NodeCommunities))
	for n, cs := range result.NodeCommunities {
		cp := make([]int, len(cs))
		copy(cp, cs)
		nodeCommunities[n] = cp
	}

	// Identify candidates.
	type candidate struct{ idx, size int }
	var candidates []candidate
	for i, c := range communities {
		if len(c) > 0 && len(c) < threshold {
			candidates = append(candidates, candidate{i, len(c)})
		}
	}
	if len(candidates) == 0 {
		return result, nil
	}
	// Sort smallest-first.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && (candidates[j].size < candidates[j-1].size ||
			(candidates[j].size == candidates[j-1].size && candidates[j].idx < candidates[j-1].idx)); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	for _, cand := range candidates {
		src := communities[cand.idx]
		if len(src) == 0 {
			continue // already consumed
		}

		// Find neighbour community with most shared nodes.
		overlap := make(map[int]int)
		for _, n := range src {
			for _, ci := range nodeCommunities[n] {
				if ci != cand.idx {
					overlap[ci]++
				}
			}
		}
		if len(overlap) == 0 {
			// No overlap — fall back to graph-connectivity neighbour search.
			connWeight := make(map[int]float64)
			for _, n := range src {
				for _, e := range g.Neighbors(n) {
					for _, ci := range nodeCommunities[e.To] {
						if ci != cand.idx {
							connWeight[ci] += e.Weight
						}
					}
				}
			}
			if len(connWeight) == 0 {
				continue // isolated — leave in place
			}
			best := -1
			bestW := -1.0
			for ci, w := range connWeight {
				if w > bestW || (w == bestW && ci < best) {
					best, bestW = ci, w
				}
			}
			target := best
			// Union.
			existing := make(map[NodeID]struct{}, len(communities[target]))
			for _, n := range communities[target] {
				existing[n] = struct{}{}
			}
			for _, n := range src {
				if _, ok := existing[n]; !ok {
					communities[target] = append(communities[target], n)
					nodeCommunities[n] = append(nodeCommunities[n], target)
				}
			}
			communities[cand.idx] = nil
			continue
		}

		// Pick target with maximum overlap (tie-break by index).
		target, bestOverlap := -1, -1
		for ci, cnt := range overlap {
			if cnt > bestOverlap || (cnt == bestOverlap && ci < target) {
				target, bestOverlap = ci, cnt
			}
		}

		// Union src into target.
		existing := make(map[NodeID]struct{}, len(communities[target]))
		for _, n := range communities[target] {
			existing[n] = struct{}{}
		}
		for _, n := range src {
			if _, ok := existing[n]; !ok {
				communities[target] = append(communities[target], n)
				nodeCommunities[n] = append(nodeCommunities[n], target)
			}
		}
		communities[cand.idx] = nil
	}

	// Compact: remove nil slots and rebuild NodeCommunities with new indices.
	var compacted [][]NodeID
	remap := make(map[int]int)
	for i, c := range communities {
		if c != nil {
			remap[i] = len(compacted)
			compacted = append(compacted, c)
		}
	}

	newNodeComm := make(map[NodeID][]int, len(nodeCommunities))
	for n, cs := range nodeCommunities {
		var newCS []int
		seen := make(map[int]struct{})
		for _, ci := range cs {
			if newIdx, ok := remap[ci]; ok {
				if _, dup := seen[newIdx]; !dup {
					newCS = append(newCS, newIdx)
					seen[newIdx] = struct{}{}
				}
			}
		}
		if len(newCS) > 0 {
			newNodeComm[n] = newCS
		}
	}

	return OverlappingCommunityResult{
		Communities:     compacted,
		NodeCommunities: newNodeComm,
	}, nil
}
```

**Step 4: Run tests**

```
go test -run TestMergeSmallOverlapping ./graph/... -v
```
Expected: all PASS

**Step 5: Commit**

```bash
git add graph/merge.go graph/merge_test.go
git commit -m "feat(merge): implement MergeSmallOverlappingCommunities"
```

---

### Task 5: Full test suite + race detector

**Files:**
- Modify: `graph/merge_test.go` (add example test)

**Step 1: Add example test (documents public API)**

Add to `graph/merge_test.go`:

```go
func ExampleMergeSmallCommunities() {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(0, 2, 1.0)
	g.AddEdge(0, 3, 1.0)

	// After Louvain detection each leaf becomes its own community.
	louvain := NewLouvain(LouvainOptions{Seed: 1})
	detected, _ := louvain.Detect(g)

	merged, _ := MergeSmallCommunities(g, detected, MergeOptions{MinSize: 2})
	_ = merged // use merged.Partition for downstream GraphRAG pipeline
}
```

**Step 2: Run full suite with race detector**

```
go test -race ./graph/... 
```
Expected: all PASS, no data races.

**Step 3: Run benchmarks (sanity check — no regression)**

```
go test -bench=. -benchtime=3s -count=1 ./graph/... | grep -E "Benchmark|ok"
```
Expected: existing benchmarks pass within ~10% of prior results.

**Step 4: Commit**

```bash
git add graph/merge_test.go
git commit -m "test(merge): add example test; verify race-free and no benchmark regression"
```

---

### Task 6: Fix the `sortCandidates` type issue (if needed)

> This task only applies if Task 3 Step 2 revealed a type mismatch with `sortCandidates`.

**Files:**
- Modify: `graph/merge.go`

The anonymous-struct parameter in `sortCandidates` won't accept the named `candidate` type. Change the helper signature and all call sites:

```go
type candidate struct {
	comm int
	size int
}

func sortCandidates(cs []candidate) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && (cs[j].size < cs[j-1].size ||
			(cs[j].size == cs[j-1].size && cs[j].comm < cs[j-1].comm)); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
```

Run:
```
go test ./graph/... 
```
Expected: PASS

Commit:
```bash
git add graph/merge.go
git commit -m "fix(merge): use named candidate type in sortCandidates"
```
