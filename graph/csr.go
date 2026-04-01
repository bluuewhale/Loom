package graph

// csrGraph is a lightweight indexed adjacency view over an existing Graph.
// It assigns each node a dense 0-based index and exposes O(1) slice-indexed
// neighbor lookups for the phase1 inner loop.
//
// Key design choices:
//   - adjByIdx holds direct references to g.adjacency[id] slices — NO edge copy.
//   - strengthByIdx precomputes node strength once at build time.
//   - nodeIDs is g.Nodes() — the already-cached sorted slice, reused by reference.
//   - idToIdx maps NodeID -> dense index for the partition/community maps that
//     still key on NodeID (state.partition, state.commStr).
//
// This is an internal view; not stored on Graph or exposed in the public API.
type csrGraph struct {
	adjByIdx      [][]Edge         // adjByIdx[i] = g.adjacency[nodeIDs[i]] — no copy
	strengthByIdx []float64        // precomputed sum of edge weights for dense index i
	nodeIDs       []NodeID         // nodeIDs[i] = NodeID at dense index i (== g.Nodes())
	idToIdx       map[NodeID]int32 // NodeID -> dense index
}

// buildCSR constructs a CSR view from g. Uses g.Nodes() (cached sorted slice).
// Does NOT copy edge data — adjByIdx slices point directly into g.adjacency.
func buildCSR(g *Graph) csrGraph {
	nodes := g.Nodes() // cached, sorted — no allocation if cache is warm
	n := len(nodes)

	idToIdx := make(map[NodeID]int32, n)
	adjByIdx := make([][]Edge, n)
	strengthByIdx := make([]float64, n)

	for i, id := range nodes {
		idx := int32(i)
		idToIdx[id] = idx
		edges := g.adjacency[id] // direct reference, no copy
		adjByIdx[i] = edges
		var s float64
		for _, e := range edges {
			s += e.Weight
		}
		strengthByIdx[i] = s
	}

	return csrGraph{
		adjByIdx:      adjByIdx,
		strengthByIdx: strengthByIdx,
		nodeIDs:       nodes,
		idToIdx:       idToIdx,
	}
}

// neighbors returns the edge slice for the node at dense index idx.
// Returns a direct reference into g.adjacency — zero allocation.
func (c *csrGraph) neighbors(idx int32) []Edge {
	return c.adjByIdx[idx]
}

// strength returns the precomputed sum of edge weights for the node at dense index idx.
func (c *csrGraph) strength(idx int32) float64 {
	return c.strengthByIdx[idx]
}
