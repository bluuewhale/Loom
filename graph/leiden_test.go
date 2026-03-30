package graph

import (
	"errors"
	"math"
	"testing"

	"github.com/bluuewhale/loom/graph/testdata"
)

// buildKarateClubLeiden creates an undirected Karate Club graph.
// Identical to buildKarateClubLouvain — same fixture, separate helper for clarity.
func buildKarateClubLeiden() *Graph {
	g := NewGraph(false)
	for _, e := range testdata.KarateClubEdges {
		g.AddEdge(NodeID(e[0]), NodeID(e[1]), 1.0)
	}
	return g
}

// TestLeidenKarateClubAccuracy verifies Leiden on the 34-node Karate Club graph:
// Q > 0.35, NMI >= 0.7 vs ground-truth 2-community partition, 2-4 communities, 34 nodes.
// Seed=2 is used: yields 3 communities with Q=0.37 and NMI=0.72 against ground truth.
func TestLeidenKarateClubAccuracy(t *testing.T) {
	g := buildKarateClubLeiden()
	det := NewLeiden(LeidenOptions{Seed: 2})
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

	// Convert ground-truth map[int]int to map[NodeID]int for NMI comparison.
	gt := make(map[NodeID]int, len(testdata.KarateClubPartition))
	for k, v := range testdata.KarateClubPartition {
		gt[NodeID(k)] = v
	}
	score := nmi(res.Partition, gt)
	if score < 0.7 {
		t.Errorf("NMI = %.4f, want >= 0.7", score)
	}
	t.Logf("KarateClub: Q=%.4f communities=%d passes=%d moves=%d NMI=%.4f",
		res.Modularity, communities, res.Passes, res.Moves, score)
}

// TestLeidenConnectedCommunities verifies that every community in the Leiden output
// is internally connected — the key correctness guarantee of the Leiden algorithm.
func TestLeidenConnectedCommunities(t *testing.T) {
	g := buildKarateClubLeiden()
	det := NewLeiden(LeidenOptions{Seed: 2})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Group nodes by community.
	commNodes := make(map[int][]NodeID)
	for node, comm := range res.Partition {
		commNodes[comm] = append(commNodes[comm], node)
	}

	// For each community, BFS using only intra-community edges and verify full coverage.
	for comm, nodes := range commNodes {
		if len(nodes) <= 1 {
			continue // singleton communities are trivially connected
		}

		inComm := make(map[NodeID]struct{}, len(nodes))
		for _, n := range nodes {
			inComm[n] = struct{}{}
		}

		visited := make(map[NodeID]bool, len(nodes))
		queue := []NodeID{nodes[0]}
		visited[nodes[0]] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, e := range g.Neighbors(cur) {
				if e.To == cur {
					continue // skip self-loops
				}
				if _, ok := inComm[e.To]; !ok {
					continue // skip cross-community edges
				}
				if !visited[e.To] {
					visited[e.To] = true
					queue = append(queue, e.To)
				}
			}
		}

		if len(visited) != len(nodes) {
			t.Errorf("community %d is disconnected: BFS reached %d/%d nodes",
				comm, len(visited), len(nodes))
		}
	}
}

// TestLeidenEmptyGraph verifies empty graph returns empty partition with nil error.
func TestLeidenEmptyGraph(t *testing.T) {
	g := NewGraph(false)
	det := NewLeiden(LeidenOptions{Seed: 1})
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

// TestLeidenSingleNode verifies single-node graph: partition has 1 entry, community=0, Q=0, Passes=1, Moves=0.
func TestLeidenSingleNode(t *testing.T) {
	g := NewGraph(false)
	g.AddNode(NodeID(7), 1.0)
	det := NewLeiden(LeidenOptions{Seed: 1})
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

// TestLeidenDirectedGraph verifies directed graph returns ErrDirectedNotSupported.
func TestLeidenDirectedGraph(t *testing.T) {
	g := NewGraph(true)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	det := NewLeiden(LeidenOptions{})
	_, err := det.Detect(g)
	if !errors.Is(err, ErrDirectedNotSupported) {
		t.Errorf("err = %v, want ErrDirectedNotSupported", err)
	}
}

// TestLeidenDisconnectedNodes verifies 5 isolated nodes each stay in own community, Q=0.
func TestLeidenDisconnectedNodes(t *testing.T) {
	g := NewGraph(false)
	for i := 0; i < 5; i++ {
		g.AddNode(NodeID(i), 1.0)
	}
	det := NewLeiden(LeidenOptions{Seed: 1})
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

// TestLeidenTwoNodeGraph verifies 2-node connected graph returns valid result, both in same community.
func TestLeidenTwoNodeGraph(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	det := NewLeiden(LeidenOptions{Seed: 1})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 2 {
		t.Errorf("partition len = %d, want 2", len(res.Partition))
	}
	// Both nodes should end up in the same community.
	if res.Partition[NodeID(0)] != res.Partition[NodeID(1)] {
		t.Errorf("two-node: nodes in different communities, expected same")
	}
}

// TestLeidenDeterministic verifies two runs with same Seed produce identical Q and community count.
func TestLeidenDeterministic(t *testing.T) {
	g := buildKarateClubLeiden()
	opts := LeidenOptions{Seed: 2}
	det := NewLeiden(opts)
	res1, err1 := det.Detect(g)
	res2, err2 := det.Detect(g)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if math.Abs(res1.Modularity-res2.Modularity) > 1e-10 {
		t.Errorf("modularity differs: %.20f vs %.20f", res1.Modularity, res2.Modularity)
	}
	c1 := uniqueCommunities(res1.Partition)
	c2 := uniqueCommunities(res2.Partition)
	if c1 != c2 {
		t.Errorf("community count differs: %d vs %d", c1, c2)
	}
}
