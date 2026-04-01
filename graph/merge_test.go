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
		g.AddEdge(NodeID(i), NodeID((i+1)%8), 1.0)
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
	// Small community: node 0 (comm 0, size 1)
	// Target A: nodes 1,2,3 (comm 1) — 2 edges to node 0
	// Target B: nodes 4,5,6,7,8,9 (comm 2) — 1 edge to node 0
	// With MergeByModularity, node 0 should prefer comm 1 (higher ΔQ from connectivity).
	g := NewGraph(false)
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
	// Node 0 merged somewhere → 2 communities total.
	if uniqueCommunities(got.Partition) != 2 {
		t.Fatalf("expected 2 communities after merge, got %d", uniqueCommunities(got.Partition))
	}
}
