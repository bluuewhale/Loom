package graph

import (
	"errors"
	"math"
	"sort"
	"testing"

	"github.com/bluuewhale/loom/graph/testdata"
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

// TestLouvainDeterministic verifies that the algorithm is reproducible: two calls
// with the same options on the same graph produce the same Modularity and community count.
// We use MaxPasses=2 which provides stable convergence on the Karate Club graph.
func TestLouvainDeterministic(t *testing.T) {
	g := buildKarateClubLouvain()
	opts := LouvainOptions{Seed: 42, MaxPasses: 2}
	det := NewLouvain(opts)
	res1, err1 := det.Detect(g)
	res2, err2 := det.Detect(g)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	// Use tolerance comparison: Q values may differ by floating-point rounding (~1e-14).
	if math.Abs(res1.Modularity-res2.Modularity) > 1e-10 {
		t.Errorf("modularity differs: %.20f vs %.20f", res1.Modularity, res2.Modularity)
	}
	c1 := uniqueCommunities(res1.Partition)
	c2 := uniqueCommunities(res2.Partition)
	if c1 != c2 {
		t.Errorf("community count differs: %d vs %d", c1, c2)
	}
	if res1.Modularity <= 0.35 {
		t.Errorf("Q = %.4f, want > 0.35", res1.Modularity)
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

// TestLouvainGiantPlusSingletons verifies a 3-node triangle plus 2 isolated nodes.
// Triangle nodes should end up in the same community; singletons each in own community.
func TestLouvainGiantPlusSingletons(t *testing.T) {
	g := NewGraph(false)
	// Triangle: nodes 0, 1, 2
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 1.0)
	g.AddEdge(NodeID(0), NodeID(2), 1.0)
	// Singletons: nodes 3 and 4
	g.AddNode(NodeID(3), 1.0)
	g.AddNode(NodeID(4), 1.0)

	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 5 {
		t.Errorf("partition len = %d, want 5", len(res.Partition))
	}
	// Triangle nodes must be in the same community.
	c0 := res.Partition[NodeID(0)]
	c1 := res.Partition[NodeID(1)]
	c2 := res.Partition[NodeID(2)]
	if c0 != c1 || c1 != c2 {
		t.Errorf("triangle nodes in different communities: %d %d %d, want same", c0, c1, c2)
	}
	// Singletons must each be in their own community.
	c3 := res.Partition[NodeID(3)]
	c4 := res.Partition[NodeID(4)]
	if c3 == c0 || c4 == c0 || c3 == c4 {
		t.Errorf("singletons not isolated: triangle=%d, s3=%d, s4=%d", c0, c3, c4)
	}
	t.Logf("GiantPlusSingletons: Q=%.4f communities=%d", res.Modularity, uniqueCommunities(res.Partition))
}

// TestLouvainZeroResolution verifies Resolution=0.0 defaults to 1.0 behavior (same as default).
// Also tests Resolution=0.001 merges into fewer communities than default.
func TestLouvainZeroResolution(t *testing.T) {
	g := buildKarateClubLouvain()

	// Resolution=0.0 should behave identically to Resolution=1.0 (default).
	detZero := NewLouvain(LouvainOptions{Seed: 42, Resolution: 0.0})
	detDefault := NewLouvain(LouvainOptions{Seed: 42})
	resZero, err := detZero.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error (zero): %v", err)
	}
	resDefault, err := detDefault.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error (default): %v", err)
	}
	if math.Abs(resZero.Modularity-resDefault.Modularity) > 1e-10 {
		t.Errorf("Resolution=0.0 Q=%.10f, default Q=%.10f, want identical", resZero.Modularity, resDefault.Modularity)
	}

	// Resolution=0.001 (very low) should produce fewer or equal communities than default.
	detLow := NewLouvain(LouvainOptions{Seed: 42, Resolution: 0.001})
	resLow, err := detLow.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error (low): %v", err)
	}
	commDefault := uniqueCommunities(resDefault.Partition)
	commLow := uniqueCommunities(resLow.Partition)
	if commLow > commDefault {
		t.Errorf("low resolution has more communities (%d) than default (%d); expect fewer", commLow, commDefault)
	}
	t.Logf("ZeroResolution: default_Q=%.4f default_comm=%d low_Q=%.4f low_comm=%d",
		resDefault.Modularity, commDefault, resLow.Modularity, commLow)
}

// TestLouvainCompleteGraph verifies a 5-node complete graph returns no error, valid partition,
// and Q close to 0 (merging all into one community).
func TestLouvainCompleteGraph(t *testing.T) {
	g := NewGraph(false)
	// Complete graph K5: 10 edges
	for i := 0; i < 5; i++ {
		for j := i + 1; j < 5; j++ {
			g.AddEdge(NodeID(i), NodeID(j), 1.0)
		}
	}

	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 5 {
		t.Errorf("partition len = %d, want 5", len(res.Partition))
	}
	// All nodes should be in the same community (Q = 0 for complete graph).
	comms := uniqueCommunities(res.Partition)
	if comms != 1 {
		t.Errorf("communities = %d, want 1 (complete graph merges all)", comms)
	}
	// Q for a complete graph is 0 regardless of partition.
	if math.Abs(res.Modularity) > 0.1 {
		t.Errorf("Q = %.4f, want ~0 for complete graph", res.Modularity)
	}
	t.Logf("CompleteGraph: Q=%.4f communities=%d", res.Modularity, comms)
}

// TestLouvainSelfLoop verifies a graph with a self-loop plus a normal edge returns no error
// and produces a valid partition.
func TestLouvainSelfLoop(t *testing.T) {
	g := NewGraph(false)
	// Self-loop on node 0
	g.AddEdge(NodeID(0), NodeID(0), 1.0)
	// Normal edge 0-1
	g.AddEdge(NodeID(0), NodeID(1), 1.0)

	det := NewLouvain(LouvainOptions{Seed: 42})
	res, err := det.Detect(g)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Partition) != 2 {
		t.Errorf("partition len = %d, want 2", len(res.Partition))
	}
	// Both nodes must have a valid community assignment.
	if _, ok := res.Partition[NodeID(0)]; !ok {
		t.Errorf("node 0 missing from partition")
	}
	if _, ok := res.Partition[NodeID(1)]; !ok {
		t.Errorf("node 1 missing from partition")
	}
	t.Logf("SelfLoop: Q=%.4f communities=%d", res.Modularity, uniqueCommunities(res.Partition))
}
