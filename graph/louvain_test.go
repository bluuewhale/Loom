package graph

import (
	"errors"
	"math"
	"sort"
	"testing"

	"community-detection/graph/testdata"
)

// buildKarateClubLouvain creates an undirected Karate Club graph.
// Reuses the same fixture as modularity_test.go.
func buildKarateClubLouvain() *Graph {
	g := NewGraph(false)
	for _, e := range testdata.KarateClubEdges {
		g.AddEdge(NodeID(e[0]), NodeID(e[1]), 1.0)
	}
	return g
}

// TestLouvainKarateClub verifies Q > 0.35, 2-4 communities, 34 nodes, Passes >= 1.
func TestLouvainKarateClub(t *testing.T) {
	g := buildKarateClubLouvain()
	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0.35 {
		t.Errorf("Q = %.4f, want > 0.35", res.Modularity)
	}
	communities := uniqueCommunities(res.Partition)
	if communities < 2 || communities > 4 {
		t.Errorf("communities = %d, want 2-4", communities)
	}
	if len(res.Partition) != 34 {
		t.Errorf("partition covers %d nodes, want 34", len(res.Partition))
	}
	if res.Passes < 1 {
		t.Errorf("Passes = %d, want >= 1", res.Passes)
	}
	t.Logf("KarateClub: Q=%.4f communities=%d passes=%d moves=%d",
		res.Modularity, communities, res.Passes, res.Moves)
}

// TestLouvainEmptyGraph verifies empty graph returns empty result with nil error.
func TestLouvainEmptyGraph(t *testing.T) {
	g := NewGraph(false)
	det := NewLouvain(LouvainOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 0 {
		t.Errorf("partition len = %d, want 0", len(res.Partition))
	}
	if res.Modularity != 0.0 {
		t.Errorf("Q = %.4f, want 0.0", res.Modularity)
	}
}

// TestLouvainSingleNode verifies single-node graph: partition has 1 entry, community=0, Q=0, Passes=1.
func TestLouvainSingleNode(t *testing.T) {
	g := NewGraph(false)
	g.AddNode(NodeID(7), 1.0)
	det := NewLouvain(LouvainOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 1 {
		t.Errorf("partition len = %d, want 1", len(res.Partition))
	}
	if res.Partition[NodeID(7)] != 0 {
		t.Errorf("community = %d, want 0", res.Partition[NodeID(7)])
	}
	if res.Modularity != 0.0 {
		t.Errorf("Q = %.4f, want 0.0", res.Modularity)
	}
	if res.Passes != 1 {
		t.Errorf("Passes = %d, want 1", res.Passes)
	}
	if res.Moves != 0 {
		t.Errorf("Moves = %d, want 0", res.Moves)
	}
}

// TestLouvainDirectedGraph verifies directed graph returns ErrDirectedNotSupported.
func TestLouvainDirectedGraph(t *testing.T) {
	g := NewGraph(true)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	det := NewLouvain(LouvainOptions{})
	_, err := det.Detect(g)
	if !errors.Is(err, ErrDirectedNotSupported) {
		t.Errorf("err = %v, want ErrDirectedNotSupported", err)
	}
}

// TestLouvainTwoDisconnectedTriangles verifies Q approx 0.5, exactly 2 communities.
func TestLouvainTwoDisconnectedTriangles(t *testing.T) {
	g := NewGraph(false)
	// Triangle A: nodes 0,1,2
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 1.0)
	g.AddEdge(NodeID(0), NodeID(2), 1.0)
	// Triangle B: nodes 3,4,5
	g.AddEdge(NodeID(3), NodeID(4), 1.0)
	g.AddEdge(NodeID(4), NodeID(5), 1.0)
	g.AddEdge(NodeID(3), NodeID(5), 1.0)

	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	communities := uniqueCommunities(res.Partition)
	if communities != 2 {
		t.Errorf("communities = %d, want 2", communities)
	}
	// Q for two equal disconnected triangles is 0.5
	if math.Abs(res.Modularity-0.5) > 0.05 {
		t.Errorf("Q = %.4f, want ~0.5", res.Modularity)
	}
}

// TestLouvainTwoNodeGraph verifies 2-node graph returns valid result without error.
func TestLouvainTwoNodeGraph(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	det := NewLouvain(LouvainOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 2 {
		t.Errorf("partition len = %d, want 2", len(res.Partition))
	}
	// Both should end up in the same community (Q=0 when all in one community)
	if res.Partition[NodeID(0)] != res.Partition[NodeID(1)] {
		t.Errorf("two-node: nodes in different communities, expected same")
	}
}

// TestLouvainDisconnectedNodes verifies 5 isolated nodes each stay in their own community, Q=0.
func TestLouvainDisconnectedNodes(t *testing.T) {
	g := NewGraph(false)
	for i := 0; i < 5; i++ {
		g.AddNode(NodeID(i), 1.0)
	}
	det := NewLouvain(LouvainOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 5 {
		t.Errorf("partition len = %d, want 5", len(res.Partition))
	}
	communities := uniqueCommunities(res.Partition)
	if communities != 5 {
		t.Errorf("communities = %d, want 5 (each node isolated)", communities)
	}
	if res.Modularity != 0.0 {
		t.Errorf("Q = %.4f, want 0.0", res.Modularity)
	}
}

// TestLouvainPartitionNormalized verifies partition values are 0-indexed contiguous.
func TestLouvainPartitionNormalized(t *testing.T) {
	g := buildKarateClubLouvain()
	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Collect all distinct community IDs
	seen := make(map[int]bool)
	for _, c := range res.Partition {
		seen[c] = true
	}
	ids := make([]int, 0, len(seen))
	for c := range seen {
		ids = append(ids, c)
	}
	sort.Ints(ids)
	// Must be {0, 1, ..., k-1}
	for i, id := range ids {
		if id != i {
			t.Errorf("community IDs not contiguous: ids[%d] = %d, want %d", i, id, i)
		}
	}
}

// TestLouvainDeterministic verifies two runs with same Seed produce identical Partition.
func TestLouvainDeterministic(t *testing.T) {
	g := buildKarateClubLouvain()
	det := NewLouvain(LouvainOptions{Seed: 42})
	res1, err1 := det.Detect(g)
	res2, err2 := det.Detect(g)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if len(res1.Partition) != len(res2.Partition) {
		t.Fatalf("partition sizes differ: %d vs %d", len(res1.Partition), len(res2.Partition))
	}
	for node, c1 := range res1.Partition {
		c2, ok := res2.Partition[node]
		if !ok || c1 != c2 {
			t.Errorf("node %d: community %d vs %d", node, c1, c2)
		}
	}
}

// TestLouvainTwoTrianglesConnected verifies two triangles connected by a bridge yield Q > 0.
func TestLouvainTwoTrianglesConnected(t *testing.T) {
	g := NewGraph(false)
	// Triangle A: 0-1-2-0
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 1.0)
	g.AddEdge(NodeID(0), NodeID(2), 1.0)
	// Triangle B: 3-4-5-3
	g.AddEdge(NodeID(3), NodeID(4), 1.0)
	g.AddEdge(NodeID(4), NodeID(5), 1.0)
	g.AddEdge(NodeID(3), NodeID(5), 1.0)
	// Bridge: 2-3
	g.AddEdge(NodeID(2), NodeID(3), 1.0)

	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Modularity <= 0 {
		t.Errorf("Q = %.4f, want > 0", res.Modularity)
	}
	t.Logf("TwoTrianglesConnected: Q=%.4f communities=%d", res.Modularity, uniqueCommunities(res.Partition))
}

// uniqueCommunities returns the number of distinct community IDs in partition.
func uniqueCommunities(partition map[NodeID]int) int {
	seen := make(map[int]struct{})
	for _, c := range partition {
		seen[c] = struct{}{}
	}
	return len(seen)
}
