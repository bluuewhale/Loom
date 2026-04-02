package graph

// idxEdge stores a neighbor's dense CSR index and edge weight.
// Used in adjIdxFlat for zero-map-lookup neighbor iteration in phase1 and BFS refinement.
type idxEdge struct {
	ToIdx  int32
	Weight float64
}

// csrGraph is a lightweight indexed adjacency view over an existing Graph.
// It assigns each node a dense 0-based index and exposes O(1) slice-indexed
// neighbor lookups for the phase1 inner loop.
//
// Key design choices:
//   - adjByIdx holds direct references to g.adjacency[id] slices — NO edge copy.
//   - adjIdxFlat + adjIdxOffsets: flat CSR of (neighbor-idx, weight) pairs —
//     eliminates all map lookups from the phase1 and BFS hot loops.
//   - strengthByIdx precomputes node strength once at build time.
//   - nodeIDs is g.Nodes() — the already-cached sorted slice, reused by reference.
//   - idToIdx maps NodeID -> dense index; nil when denseIDs=true (supergraph fast-path).
//   - denseIDs=true when nodes are NodeID(0)..NodeID(n-1) (always true for supergraphs),
//     allowing idToIdx lookup to be replaced by direct integer cast — zero map allocation.
//
// This is an internal view; not stored on Graph or exposed in the public API.
type csrGraph struct {
	adjByIdx      [][]Edge         // adjByIdx[i] = g.adjacency[nodeIDs[i]] — no copy
	adjIdxFlat    []idxEdge        // flat array: (neighbor-idx, weight) for every edge, all nodes
	adjIdxOffsets []int32          // adjIdxOffsets[i..i+1] gives node i's range in adjIdxFlat
	strengthByIdx []float64        // precomputed sum of edge weights for dense index i
	nodeIDs       []NodeID         // nodeIDs[i] = NodeID at dense index i (== g.Nodes())
	idToIdx       map[NodeID]int32 // NodeID -> dense index; nil when denseIDs is true
	denseIDs      bool             // true when nodeIDs == [NodeID(0), NodeID(1), ..., NodeID(n-1)]
}

// nodeToIdx returns the dense CSR index for a node. Uses direct cast when denseIDs is
// true (supergraph fast-path), falling back to the idToIdx map for arbitrary NodeIDs.
func (c *csrGraph) nodeToIdx(id NodeID) int32 {
	if c.denseIDs {
		return int32(id)
	}
	return c.idToIdx[id]
}

// buildCSR constructs a CSR view from g. Uses g.Nodes() (cached sorted slice).
// Does NOT copy edge data in adjByIdx — those slices point directly into g.adjacency.
// adjIdxFlat is a single allocation covering all edges; adjIdxOffsets is n+1 int32s.
// When node IDs are contiguous [0, n-1] (always true for supergraphs), the idToIdx map
// is omitted entirely — NodeID casts serve as O(1) index lookups with zero allocation.
func buildCSR(g *Graph) csrGraph {
	nodes := g.Nodes() // cached, sorted — no allocation if cache is warm
	n := len(nodes)

	// Detect dense-ID layout: nodes are NodeID(0)..NodeID(n-1) iff first and last match.
	// g.Nodes() is sorted, so checking endpoints is sufficient.
	dense := n > 0 && nodes[0] == NodeID(0) && nodes[n-1] == NodeID(n-1)

	adjByIdx := make([][]Edge, n)
	strengthByIdx := make([]float64, n)

	var idToIdx map[NodeID]int32
	if !dense {
		idToIdx = make(map[NodeID]int32, n)
	}

	totalEdges := 0
	for i, id := range nodes {
		if !dense {
			idToIdx[id] = int32(i)
		}
		edges := g.adjacency[id] // direct reference, no copy
		adjByIdx[i] = edges
		var s float64
		for _, e := range edges {
			s += e.Weight
		}
		strengthByIdx[i] = s
		totalEdges += len(edges)
	}

	// Build flat idx-edge array and offset table in a single pass.
	// Two allocations cover all nodes, eliminating per-node inner-slice allocs.
	adjIdxFlat := make([]idxEdge, totalEdges)
	adjIdxOffsets := make([]int32, n+1)
	pos := 0
	if dense {
		for i, id := range nodes {
			adjIdxOffsets[i] = int32(pos)
			for _, e := range g.adjacency[id] {
				adjIdxFlat[pos] = idxEdge{ToIdx: int32(e.To), Weight: e.Weight}
				pos++
			}
		}
	} else {
		for i, id := range nodes {
			adjIdxOffsets[i] = int32(pos)
			for _, e := range g.adjacency[id] {
				adjIdxFlat[pos] = idxEdge{ToIdx: idToIdx[e.To], Weight: e.Weight}
				pos++
			}
		}
	}
	adjIdxOffsets[n] = int32(pos)

	return csrGraph{
		adjByIdx:      adjByIdx,
		adjIdxFlat:    adjIdxFlat,
		adjIdxOffsets: adjIdxOffsets,
		strengthByIdx: strengthByIdx,
		nodeIDs:       nodes,
		idToIdx:       idToIdx,
		denseIDs:      dense,
	}
}

// buildCSRInto is like buildCSR but fills dst in-place, reusing its existing slice
// capacities. On the first call dst is empty (zero-value csrGraph) and behaves
// identically to buildCSR. On subsequent calls the slices grow only when needed,
// eliminating 4 allocations per CSR build after pool warm-up.
//
// Callers must treat dst as exclusively owned for the lifetime of the csrGraph view.
func buildCSRInto(g *Graph, dst *csrGraph) {
	nodes := g.Nodes()
	n := len(nodes)

	dense := n > 0 && nodes[0] == NodeID(0) && nodes[n-1] == NodeID(n-1)

	// Grow adjByIdx and strengthByIdx lazily.
	if cap(dst.adjByIdx) >= n {
		dst.adjByIdx = dst.adjByIdx[:n]
	} else {
		dst.adjByIdx = make([][]Edge, n)
	}
	if cap(dst.strengthByIdx) >= n {
		dst.strengthByIdx = dst.strengthByIdx[:n]
	} else {
		dst.strengthByIdx = make([]float64, n)
	}

	var idToIdx map[NodeID]int32
	if !dense {
		if dst.idToIdx == nil {
			dst.idToIdx = make(map[NodeID]int32, n)
		} else {
			clear(dst.idToIdx)
		}
		idToIdx = dst.idToIdx
	}

	totalEdges := 0
	for i, id := range nodes {
		if !dense {
			idToIdx[id] = int32(i)
		}
		edges := g.adjacency[id]
		dst.adjByIdx[i] = edges
		var s float64
		for _, e := range edges {
			s += e.Weight
		}
		dst.strengthByIdx[i] = s
		totalEdges += len(edges)
	}

	if cap(dst.adjIdxFlat) >= totalEdges {
		dst.adjIdxFlat = dst.adjIdxFlat[:totalEdges]
	} else {
		dst.adjIdxFlat = make([]idxEdge, totalEdges)
	}
	if cap(dst.adjIdxOffsets) >= n+1 {
		dst.adjIdxOffsets = dst.adjIdxOffsets[:n+1]
	} else {
		dst.adjIdxOffsets = make([]int32, n+1)
	}

	pos := 0
	if dense {
		for i, id := range nodes {
			dst.adjIdxOffsets[i] = int32(pos)
			for _, e := range g.adjacency[id] {
				dst.adjIdxFlat[pos] = idxEdge{ToIdx: int32(e.To), Weight: e.Weight}
				pos++
			}
		}
	} else {
		for i, id := range nodes {
			dst.adjIdxOffsets[i] = int32(pos)
			for _, e := range g.adjacency[id] {
				dst.adjIdxFlat[pos] = idxEdge{ToIdx: idToIdx[e.To], Weight: e.Weight}
				pos++
			}
		}
	}
	dst.adjIdxOffsets[n] = int32(pos)

	dst.nodeIDs = nodes
	dst.denseIDs = dense
}

// neighbors returns the edge slice for the node at dense index idx.
// Returns a direct reference into g.adjacency — zero allocation.
func (c *csrGraph) neighbors(idx int32) []Edge {
	return c.adjByIdx[idx]
}

// neighborsIdx returns the idx-edge slice for the node at dense index idx.
// Zero allocation — slice of the flat array.
func (c *csrGraph) neighborsIdx(idx int32) []idxEdge {
	return c.adjIdxFlat[c.adjIdxOffsets[idx]:c.adjIdxOffsets[idx+1]]
}

// strength returns the precomputed sum of edge weights for the node at dense index idx.
func (c *csrGraph) strength(idx int32) float64 {
	return c.strengthByIdx[idx]
}
