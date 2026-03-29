package graph

import (
	"math"
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
