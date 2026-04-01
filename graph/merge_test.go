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
