package graph

import (
	"math"
	"testing"
)

// floatEq compares two float64 values with a tight epsilon.
func floatEq(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

// containsEdge checks whether edges contains an edge to 'to' with the given weight.
func containsEdge(t *testing.T, edges []Edge, to NodeID, weight float64) {
	t.Helper()
	for _, e := range edges {
		if e.To == to && math.Abs(e.Weight-weight) < 1e-9 {
			return
		}
	}
	t.Errorf("edges %v does not contain edge to %v with weight %v", edges, to, weight)
}

// containsNode checks whether nodeIDs contains the given id.
func containsNode(t *testing.T, ids []NodeID, id NodeID) {
	t.Helper()
	for _, n := range ids {
		if n == id {
			return
		}
	}
	t.Errorf("node list %v does not contain %v", ids, id)
}

// TestNewGraph verifies NewGraph(false) creates a valid undirected graph.
func TestNewGraph(t *testing.T) {
	g := NewGraph(false)
	if g == nil {
		t.Fatal("NewGraph returned nil")
	}
	if g.IsDirected() {
		t.Error("expected undirected graph")
	}
	if g.NodeCount() != 0 {
		t.Errorf("expected NodeCount==0, got %d", g.NodeCount())
	}
}

// TestNewGraphDirected verifies NewGraph(true) creates a directed graph.
func TestNewGraphDirected(t *testing.T) {
	g := NewGraph(true)
	if !g.IsDirected() {
		t.Error("expected directed graph")
	}
}

// TestAddNodeAndQuery verifies AddNode adds a node queryable via Nodes/NodeCount.
func TestAddNodeAndQuery(t *testing.T) {
	g := NewGraph(false)
	g.AddNode(1, 1.0)
	if g.NodeCount() != 1 {
		t.Errorf("expected NodeCount==1, got %d", g.NodeCount())
	}
	containsNode(t, g.Nodes(), NodeID(1))
}

// TestAddEdgeUndirected verifies undirected edge storage and TotalWeight.
func TestAddEdgeUndirected(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 2.5)
	containsEdge(t, g.Neighbors(0), 1, 2.5)
	containsEdge(t, g.Neighbors(1), 0, 2.5)
	floatEq(t, g.TotalWeight(), 2.5)
}

// TestAddEdgeDirected verifies directed edge: only forward direction stored.
func TestAddEdgeDirected(t *testing.T) {
	g := NewGraph(true)
	g.AddEdge(0, 1, 3.0)
	containsEdge(t, g.Neighbors(0), 1, 3.0)
	if len(g.Neighbors(1)) != 0 {
		t.Errorf("expected Neighbors(1) empty for directed graph, got %v", g.Neighbors(1))
	}
	floatEq(t, g.TotalWeight(), 3.0)
}

// TestAutoCreateNodes verifies AddEdge auto-creates missing nodes.
func TestAutoCreateNodes(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(5, 10, 1.0)
	if g.NodeCount() != 2 {
		t.Errorf("expected NodeCount==2, got %d", g.NodeCount())
	}
	containsNode(t, g.Nodes(), NodeID(5))
	containsNode(t, g.Nodes(), NodeID(10))
}

// TestStrength verifies Strength returns sum of incident edge weights.
// Triangle: 0-1 (1.0), 1-2 (1.0), 0-2 (1.0) — each node has degree 2.
func TestStrength(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(0, 2, 1.0)
	floatEq(t, g.Strength(0), 2.0)
	floatEq(t, g.Strength(1), 2.0)
	floatEq(t, g.Strength(2), 2.0)
}

// TestEdgeCount verifies EdgeCount returns distinct edge count for undirected graph.
func TestEdgeCount(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 0, 1.0)
	if g.EdgeCount() != 3 {
		t.Errorf("expected EdgeCount==3, got %d", g.EdgeCount())
	}
}

// TestClone verifies Clone produces an independent deep copy.
func TestClone(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(0, 2, 1.0)
	originalCount := g.NodeCount()

	c := g.Clone()
	// Mutate clone — original must be unchanged
	c.AddNode(99, 1.0)

	if g.NodeCount() != originalCount {
		t.Errorf("original NodeCount changed after mutating clone: got %d, want %d", g.NodeCount(), originalCount)
	}
	if c.NodeCount() != originalCount+1 {
		t.Errorf("clone NodeCount expected %d, got %d", originalCount+1, c.NodeCount())
	}
	// TotalWeight preserved
	floatEq(t, c.TotalWeight(), g.TotalWeight())
}

// TestSubgraph verifies Subgraph extracts correct nodes and edges.
// 4-node path: 0-1-2-3; Subgraph([1,2]) should have 2 nodes, 1 edge.
func TestSubgraph(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 2.0)
	g.AddEdge(2, 3, 1.0)

	sub := g.Subgraph([]NodeID{1, 2})
	if sub.NodeCount() != 2 {
		t.Errorf("expected NodeCount==2, got %d", sub.NodeCount())
	}
	if sub.EdgeCount() != 1 {
		t.Errorf("expected EdgeCount==1, got %d", sub.EdgeCount())
	}
	floatEq(t, sub.TotalWeight(), 2.0)
	containsEdge(t, sub.Neighbors(1), 2, 2.0)
	containsEdge(t, sub.Neighbors(2), 1, 2.0)
}

// TestWeightToComm verifies WeightToComm sums weights to nodes in a community.
// Path 0-1-2, partition {0:0, 1:0, 2:1}
func TestWeightToComm(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.5)
	g.AddEdge(1, 2, 2.5)
	partition := map[NodeID]int{0: 0, 1: 0, 2: 1}

	floatEq(t, g.WeightToComm(1, 0, partition), 1.5) // edge 1->0
	floatEq(t, g.WeightToComm(1, 1, partition), 2.5) // edge 1->2
}

// TestCommStrength verifies CommStrength sums Strength over all nodes in a community.
func TestCommStrength(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.5)
	g.AddEdge(1, 2, 2.5)
	partition := map[NodeID]int{0: 0, 1: 0, 2: 1}

	// Strength(0) = 1.5, Strength(1) = 1.5+2.5=4.0 -> CommStrength(comm=0) = 5.5
	floatEq(t, g.CommStrength(0, partition), 5.5)
	// Strength(2) = 2.5 -> CommStrength(comm=1) = 2.5
	floatEq(t, g.CommStrength(1, partition), 2.5)
}

// TestSelfLoop verifies self-loops: stored once in adjacency, contribute to Strength and TotalWeight.
func TestSelfLoop(t *testing.T) {
	g := NewGraph(false)
	g.AddEdge(0, 0, 1.5)
	floatEq(t, g.TotalWeight(), 1.5)
	floatEq(t, g.Strength(0), 1.5)
}

// TestEmptyGraph verifies behavior on an empty graph.
func TestEmptyGraph(t *testing.T) {
	g := NewGraph(false)
	nodes := g.Nodes()
	if len(nodes) != 0 {
		t.Errorf("expected empty Nodes(), got %v", nodes)
	}
	floatEq(t, g.TotalWeight(), 0.0)
	if g.Neighbors(99) != nil {
		t.Errorf("expected Neighbors(99)==nil for empty graph")
	}
}
