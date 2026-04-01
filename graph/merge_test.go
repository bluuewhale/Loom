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
