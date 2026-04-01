package graph

// csrGraph is a Compressed Sparse Row representation of graph adjacency.
// It provides O(1) indexed neighbor lookups with contiguous memory layout
// for better cache locality in the phase1 inner loop.
//
// This is an internal view built at the start of Detect() — not stored on
// Graph or exposed in the public API.
type csrGraph struct {
	offsets []int32          // len = nodeCount+1; neighbors of dense index i are edges[offsets[i]:offsets[i+1]]
	edges   []Edge           // flat contiguous edge list
	nodeIDs []NodeID         // maps dense index -> NodeID
	idToIdx map[NodeID]int32 // maps NodeID -> dense index
}

// buildCSR constructs a CSR view from g. Uses g.Nodes() (cached sorted slice).
// The CSR maps each NodeID to a dense 0-based index for O(1) lookups.
func buildCSR(g *Graph) csrGraph {
	nodes := g.Nodes() // cached, sorted
	n := len(nodes)

	idToIdx := make(map[NodeID]int32, n)
	for i, id := range nodes {
		idToIdx[id] = int32(i)
	}

	// Compute offsets: offsets[i+1] = offsets[i] + degree(nodes[i])
	offsets := make([]int32, n+1)
	for i, id := range nodes {
		offsets[i+1] = offsets[i] + int32(len(g.adjacency[id]))
	}

	// Copy edges into flat array
	edges := make([]Edge, offsets[n])
	for i, id := range nodes {
		copy(edges[offsets[i]:offsets[i+1]], g.adjacency[id])
	}

	return csrGraph{
		offsets: offsets,
		edges:   edges,
		nodeIDs: nodes,
		idToIdx: idToIdx,
	}
}

// neighbors returns the edge slice for the node at dense index idx.
// This is a sub-slice of the flat edges array — no allocation.
func (c *csrGraph) neighbors(idx int32) []Edge {
	return c.edges[c.offsets[idx]:c.offsets[idx+1]]
}

// strength returns the sum of edge weights for the node at dense index idx.
func (c *csrGraph) strength(idx int32) float64 {
	var s float64
	for _, e := range c.edges[c.offsets[idx]:c.offsets[idx+1]] {
		s += e.Weight
	}
	return s
}
