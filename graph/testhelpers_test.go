package graph

import (
	"math"
	"math/rand"
	"slices"
)

// nmi computes normalized mutual information between two partitions.
// NMI = 2 * I(X;Y) / (H(X) + H(Y)) where I(X;Y) = H(X) + H(Y) - H(X,Y).
// Label-agnostic: measures structural agreement regardless of community ID values.
// Returns 0.0 when len(p1)==0. Returns 1.0 for identical single-community partitions.
func nmi(p1, p2 map[NodeID]int) float64 {
	n := float64(len(p1))
	if n == 0 {
		return 0.0
	}

	// Build contingency table and marginal counts.
	joint := make(map[[2]int]float64)
	cnt1 := make(map[int]float64)
	cnt2 := make(map[int]float64)
	for node, c1 := range p1 {
		c2 := p2[node]
		joint[[2]int{c1, c2}]++
		cnt1[c1]++
		cnt2[c2]++
	}

	// Compute H(X), H(Y), H(X,Y).
	hx, hy, hxy := 0.0, 0.0, 0.0
	for _, count := range cnt1 {
		p := count / n
		hx -= p * math.Log2(p)
	}
	for _, count := range cnt2 {
		p := count / n
		hy -= p * math.Log2(p)
	}
	for _, count := range joint {
		p := count / n
		hxy -= p * math.Log2(p)
	}

	mi := hx + hy - hxy
	denom := hx + hy
	if denom == 0 {
		return 1.0 // identical single-community partitions
	}
	return 2.0 * mi / denom
}

// uniqueCommunities returns the number of distinct community IDs in partition.
func uniqueCommunities(partition map[NodeID]int) int {
	seen := make(map[int]struct{})
	for _, c := range partition {
		seen[c] = struct{}{}
	}
	return len(seen)
}

// buildGraph creates an undirected graph from an edge list.
func buildGraph(edges [][2]int) *Graph {
	g := NewGraph(false)
	for _, e := range edges {
		g.AddEdge(NodeID(e[0]), NodeID(e[1]), 1.0)
	}
	return g
}

// groundTruthPartition converts a map[int]int ground-truth to map[NodeID]int.
func groundTruthPartition(gt map[int]int) map[NodeID]int {
	result := make(map[NodeID]int, len(gt))
	for k, v := range gt {
		result[NodeID(k)] = v
	}
	return result
}

// perturbGraph returns a copy of g with nRemove existing edges removed and nAdd
// new random edges added, using a seeded RNG for reproducibility.
// For undirected graphs, each undirected edge is counted once.
func perturbGraph(g *Graph, nRemove, nAdd int, seed int64) *Graph {
	rng := rand.New(rand.NewSource(seed))
	nodes := g.Nodes()
	slices.Sort(nodes)

	// Collect all undirected edges (canonical: from < to).
	type edge struct {
		from, to NodeID
		weight   float64
	}
	var allEdges []edge
	for _, n := range nodes {
		for _, e := range g.Neighbors(n) {
			if n < e.To { // canonical direction only
				allEdges = append(allEdges, edge{n, e.To, e.Weight})
			}
		}
	}

	// Select edges to remove (shuffle then take first nRemove).
	rng.Shuffle(len(allEdges), func(i, j int) {
		allEdges[i], allEdges[j] = allEdges[j], allEdges[i]
	})
	removeSet := make(map[[2]NodeID]struct{}, nRemove)
	for i := 0; i < nRemove && i < len(allEdges); i++ {
		removeSet[[2]NodeID{allEdges[i].from, allEdges[i].to}] = struct{}{}
	}

	// Rebuild graph without removed edges.
	pg := NewGraph(false)
	// Ensure all nodes exist (even if all their edges are removed).
	for _, n := range nodes {
		pg.AddNode(n, 1.0)
	}
	for _, e := range allEdges {
		key := [2]NodeID{e.from, e.to}
		if _, removed := removeSet[key]; !removed {
			pg.AddEdge(e.from, e.to, e.weight)
		}
	}

	// Add nAdd random new edges between existing nodes (skip self-loops and duplicates).
	added := 0
	for added < nAdd {
		a := nodes[rng.Intn(len(nodes))]
		b := nodes[rng.Intn(len(nodes))]
		if a != b {
			pg.AddEdge(a, b, 1.0)
			added++
		}
	}
	return pg
}

// cloneWithAdditions returns a copy of g with the given new nodes and edges applied.
// Intended for benchmark setup: creates the "post-update" graph state used alongside a GraphDelta.
func cloneWithAdditions(g *Graph, newNodes []NodeID, newEdges []DeltaEdge) *Graph {
	out := NewGraph(false)
	for _, n := range g.Nodes() {
		out.AddNode(n, 1.0)
		for _, e := range g.Neighbors(n) {
			if n < e.To {
				out.AddEdge(n, e.To, e.Weight)
			}
		}
	}
	for _, n := range newNodes {
		out.AddNode(n, 1.0)
	}
	for _, e := range newEdges {
		out.AddEdge(e.From, e.To, e.Weight)
	}
	return out
}
