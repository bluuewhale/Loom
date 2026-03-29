package graph

import (
	"math"
	"testing"

	"community-detection/graph/testdata"
)

// buildKarateClub creates an undirected Karate Club graph from the testdata fixture.
func buildKarateClub() *Graph {
	g := NewGraph(false)
	for _, e := range testdata.KarateClubEdges {
		g.AddEdge(NodeID(e[0]), NodeID(e[1]), 1.0)
	}
	return g
}

// karatePartition converts the testdata int-keyed partition to NodeID-keyed.
func karatePartition() map[NodeID]int {
	p := make(map[NodeID]int, len(testdata.KarateClubPartition))
	for k, v := range testdata.KarateClubPartition {
		p[NodeID(k)] = v
	}
	return p
}

// TestModularityKarateClub verifies that the ground-truth partition yields Q ≈ 0.371.
func TestModularityKarateClub(t *testing.T) {
	g := buildKarateClub()
	p := karatePartition()
	got := ComputeModularity(g, p)
	want := 0.371
	tol := 0.02
	if math.Abs(got-want) > tol {
		t.Errorf("Karate Club Q = %.4f, want %.3f ± %.3f", got, want, tol)
	}
	t.Logf("Karate Club Q = %.6f (want %.3f ± %.3f)", got, want, tol)
}

// TestModularitySingleCommunity checks that placing all nodes in one community gives Q == 0.0.
func TestModularitySingleCommunity(t *testing.T) {
	g := buildKarateClub()
	p := make(map[NodeID]int)
	for _, n := range g.Nodes() {
		p[n] = 0
	}
	got := ComputeModularity(g, p)
	if math.Abs(got) > 1e-9 {
		t.Errorf("single community Q = %.10f, want 0.0 (within 1e-9)", got)
	}
}

// TestModularityEachNodeOwnCommunity checks that giving each node its own community
// yields Q < 0 for a connected graph.
func TestModularityEachNodeOwnCommunity(t *testing.T) {
	g := buildKarateClub()
	p := make(map[NodeID]int)
	for i, n := range g.Nodes() {
		p[n] = i
	}
	got := ComputeModularity(g, p)
	if got >= 0 {
		t.Errorf("each-node-own-community Q = %.6f, want < 0", got)
	}
}

// TestModularitySingleNode verifies a single isolated node returns Q == 0.0.
func TestModularitySingleNode(t *testing.T) {
	g := NewGraph(false)
	g.AddNode(NodeID(0), 1.0)
	p := map[NodeID]int{NodeID(0): 0}
	got := ComputeModularity(g, p)
	if math.Abs(got) > 1e-9 {
		t.Errorf("single node Q = %.10f, want 0.0 (within 1e-9)", got)
	}
}

// TestModularitySingleEdge checks that 2 nodes + 1 edge all in one community gives Q == 0.0.
func TestModularitySingleEdge(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	p := map[NodeID]int{NodeID(0): 0, NodeID(1): 0}
	got := ComputeModularity(g, p)
	if math.Abs(got) > 1e-9 {
		t.Errorf("single edge same community Q = %.10f, want 0.0 (within 1e-9)", got)
	}
}

// TestModularitySingleEdgeTwoCommunities checks that 2 nodes + 1 edge in different communities
// yields Q ≈ -0.5.
func TestModularitySingleEdgeTwoCommunities(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	p := map[NodeID]int{NodeID(0): 0, NodeID(1): 1}
	got := ComputeModularity(g, p)
	want := -0.5
	tol := 0.01
	if math.Abs(got-want) > tol {
		t.Errorf("single edge two communities Q = %.6f, want %.2f ± %.2f", got, want, tol)
	}
}

// TestModularityWeighted verifies a weighted triangle with two communities matches
// a hand-calculated Q.
func TestModularityWeighted(t *testing.T) {
	// Triangle: edges 0-1 (w=1), 1-2 (w=2), 0-2 (w=3)
	// Total weight W = 6, twoW = 12
	// Partition: {0,1} -> comm 0, {2} -> comm 1
	// Comm 0: intraWeight = 2 * weight(0-1) = 2*1 = 2; degSum = k0+k1 = (1+3) + (1+2) = 7
	//   term0 = 2/12 - (7/12)^2 = 0.1667 - 0.3403 = -0.1736
	// Comm 1: intraWeight = 0; degSum = k2 = 2+3 = 5
	//   term1 = 0/12 - (5/12)^2 = -0.1736
	// Q = term0 + term1 ≈ -0.3472
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 2.0)
	g.AddEdge(NodeID(0), NodeID(2), 3.0)
	p := map[NodeID]int{NodeID(0): 0, NodeID(1): 0, NodeID(2): 1}

	got := ComputeModularity(g, p)
	// Compute exact expected value
	twoW := 12.0
	// comm 0: intra=2 (edge 0-1 seen from both sides), deg=4+3=7
	// comm 1: intra=0, deg=2+3=5
	want := (2.0/twoW - (7.0/twoW)*(7.0/twoW)) + (0.0/twoW - (5.0/twoW)*(5.0/twoW))
	tol := 1e-9
	if math.Abs(got-want) > tol {
		t.Errorf("weighted triangle Q = %.10f, want %.10f (diff = %.2e)", got, want, math.Abs(got-want))
	}
}

// TestModularityWeightedResolution verifies that resolution != 1.0 produces different Q.
func TestModularityWeightedResolution(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 2.0)
	g.AddEdge(NodeID(0), NodeID(2), 3.0)
	p := map[NodeID]int{NodeID(0): 0, NodeID(1): 0, NodeID(2): 1}

	q1 := ComputeModularityWeighted(g, p, 1.0)
	q15 := ComputeModularityWeighted(g, p, 1.5)
	if math.Abs(q1-q15) < 1e-9 {
		t.Errorf("resolution=1.0 Q (%.6f) == resolution=1.5 Q (%.6f), expected different values", q1, q15)
	}
}

// TestModularityEmptyGraph verifies that a graph with no nodes returns Q == 0.0.
func TestModularityEmptyGraph(t *testing.T) {
	g := NewGraph(false)
	p := map[NodeID]int{}
	got := ComputeModularity(g, p)
	if math.Abs(got) > 1e-9 {
		t.Errorf("empty graph Q = %.10f, want 0.0", got)
	}
}

// TestModularityEdgeCases covers complete graph, ring graph, and disconnected graph
// using table-driven tests.
func TestModularityEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		buildFunc func() (*Graph, map[NodeID]int)
		wantQ     float64
		tolerance float64
	}{
		{
			name: "complete K5 all same community",
			buildFunc: func() (*Graph, map[NodeID]int) {
				g := NewGraph(false)
				n := 5
				for i := 0; i < n; i++ {
					for j := i + 1; j < n; j++ {
						g.AddEdge(NodeID(i), NodeID(j), 1.0)
					}
				}
				p := make(map[NodeID]int)
				for i := 0; i < n; i++ {
					p[NodeID(i)] = 0
				}
				return g, p
			},
			wantQ:     0.0,
			tolerance: 1e-9,
		},
		{
			name: "complete K5 each own community",
			buildFunc: func() (*Graph, map[NodeID]int) {
				g := NewGraph(false)
				n := 5
				for i := 0; i < n; i++ {
					for j := i + 1; j < n; j++ {
						g.AddEdge(NodeID(i), NodeID(j), 1.0)
					}
				}
				p := make(map[NodeID]int)
				for i := 0; i < n; i++ {
					p[NodeID(i)] = i
				}
				return g, p
			},
			wantQ:     -0.2, // should be negative
			tolerance: 0.5,  // wide tolerance — just verify sign
		},
		{
			name: "ring 6 nodes two halves",
			buildFunc: func() (*Graph, map[NodeID]int) {
				// Ring: 0-1-2-3-4-5-0
				g := NewGraph(false)
				for i := 0; i < 6; i++ {
					g.AddEdge(NodeID(i), NodeID((i+1)%6), 1.0)
				}
				// Split: {0,1,2} -> 0, {3,4,5} -> 1
				p := map[NodeID]int{0: 0, 1: 0, 2: 0, 3: 1, 4: 1, 5: 1}
				return g, p
			},
			// W=6, twoW=12
			// comm0: intra= edges(0-1, 1-2) = 4 (counted twice each), deg=1+2+1=...
			// 0: neighbors 1,5 -> k0=2; 1: neighbors 0,2 -> k1=2; 2: neighbors 1,3 -> k2=2
			// comm0 deg=2+2+2=6, intra = edges within {0,1,2}: 0-1(x2), 1-2(x2) = 4
			// comm1: deg=6, intra=4 (3-4, 4-5 each twice)
			// Q = (4/12 - (6/12)^2) + (4/12 - (6/12)^2) = 2*(0.333 - 0.25) = 2*0.0833 = 0.1667
			wantQ:     1.0 / 6.0,
			tolerance: 0.01,
		},
		{
			name: "disconnected two triangles",
			buildFunc: func() (*Graph, map[NodeID]int) {
				// Triangle A: 0-1-2-0; Triangle B: 3-4-5-3
				g := NewGraph(false)
				g.AddEdge(NodeID(0), NodeID(1), 1.0)
				g.AddEdge(NodeID(1), NodeID(2), 1.0)
				g.AddEdge(NodeID(0), NodeID(2), 1.0)
				g.AddEdge(NodeID(3), NodeID(4), 1.0)
				g.AddEdge(NodeID(4), NodeID(5), 1.0)
				g.AddEdge(NodeID(3), NodeID(5), 1.0)
				p := map[NodeID]int{0: 0, 1: 0, 2: 0, 3: 1, 4: 1, 5: 1}
				return g, p
			},
			// W=6, twoW=12. Each triangle: intra=6 (3 edges x2), deg=2*3=6
			// Q = 2*(6/12 - (6/12)^2) = 2*(0.5 - 0.25) = 0.5
			wantQ:     0.5,
			tolerance: 1e-9,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, p := tc.buildFunc()
			got := ComputeModularity(g, p)
			if tc.name == "complete K5 each own community" {
				// Just verify it's negative
				if got >= 0 {
					t.Errorf("Q = %.6f, want negative", got)
				}
				return
			}
			if math.Abs(got-tc.wantQ) > tc.tolerance {
				t.Errorf("Q = %.10f, want %.10f ± %.2e", got, tc.wantQ, tc.tolerance)
			}
		})
	}
}

// TestModularityCompleteGraph is an explicit test for complete graph correctness.
func TestModularityCompleteGraph(t *testing.T) {
	n := 5
	g := NewGraph(false)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			g.AddEdge(NodeID(i), NodeID(j), 1.0)
		}
	}
	p := make(map[NodeID]int)
	for i := 0; i < n; i++ {
		p[NodeID(i)] = 0
	}
	got := ComputeModularity(g, p)
	if math.Abs(got) > 1e-9 {
		t.Errorf("complete K5 all-same-community Q = %.10f, want 0.0", got)
	}
}

// TestModularityRingGraph is an explicit test for ring graph correctness.
func TestModularityRingGraph(t *testing.T) {
	g := NewGraph(false)
	for i := 0; i < 6; i++ {
		g.AddEdge(NodeID(i), NodeID((i+1)%6), 1.0)
	}
	// Two halves: {0,1,2} and {3,4,5}
	p := map[NodeID]int{0: 0, 1: 0, 2: 0, 3: 1, 4: 1, 5: 1}
	got := ComputeModularity(g, p)
	want := 1.0 / 6.0
	tol := 0.01
	if math.Abs(got-want) > tol {
		t.Errorf("ring 6 Q = %.6f, want %.6f ± %.3f", got, want, tol)
	}
}

// TestModularityDisconnectedGraph verifies two disconnected triangles give Q ≈ 0.5.
func TestModularityDisconnectedGraph(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(NodeID(0), NodeID(1), 1.0)
	g.AddEdge(NodeID(1), NodeID(2), 1.0)
	g.AddEdge(NodeID(0), NodeID(2), 1.0)
	g.AddEdge(NodeID(3), NodeID(4), 1.0)
	g.AddEdge(NodeID(4), NodeID(5), 1.0)
	g.AddEdge(NodeID(3), NodeID(5), 1.0)
	p := map[NodeID]int{0: 0, 1: 0, 2: 0, 3: 1, 4: 1, 5: 1}
	got := ComputeModularity(g, p)
	want := 0.5
	tol := 1e-9
	if math.Abs(got-want) > tol {
		t.Errorf("two triangles Q = %.10f, want %.2f ± %.2e", got, want, tol)
	}
}

// BenchmarkComputeModularityKarate benchmarks Q computation on the Karate Club graph.
func BenchmarkComputeModularityKarate(b *testing.B) {
	g := buildKarateClub()
	p := karatePartition()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeModularity(g, p)
	}
}
