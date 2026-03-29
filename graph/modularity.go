package graph

// ComputeModularity calculates the Newman-Girvan modularity Q for the given graph
// and community partition. Q measures the density of connections within communities
// vs. connections between communities.
//
// Formula: Q = Σ_c [ intraWeight/twoW - (degSum/twoW)^2 ]
// where twoW = 2 * TotalWeight() for undirected graphs, intraWeight is the sum
// of internal edge weights in community c (each edge counted twice due to
// bidirectional adjacency), and degSum is the sum of node strengths in c.
//
// Returns 0.0 if the graph has no edges or the partition is empty.
// Nodes missing from partition are assigned to community -1 (singleton).
func ComputeModularity(g *Graph, partition map[NodeID]int) float64 {
	return ComputeModularityWeighted(g, partition, 1.0)
}

// ComputeModularityWeighted calculates the resolution-parameterized modularity Q.
// With resolution == 1.0, this is identical to ComputeModularity.
// Higher resolution values favor more and smaller communities.
//
// Formula: Q = Σ_c [ intraWeight/twoW - resolution * (degSum/twoW)^2 ]
func ComputeModularityWeighted(g *Graph, partition map[NodeID]int, resolution float64) float64 {
	var twoW float64
	if g.directed {
		twoW = g.TotalWeight()
	} else {
		twoW = 2.0 * g.TotalWeight()
	}
	if twoW == 0 {
		return 0.0
	}

	type commStats struct {
		intraWeight float64 // sum of internal edge weights (each undirected edge counted twice)
		degSum      float64 // sum of node strengths in community
	}

	stats := make(map[int]*commStats)

	for _, nid := range g.Nodes() {
		c, ok := partition[nid]
		if !ok {
			c = -1
		}
		if stats[c] == nil {
			stats[c] = &commStats{}
		}
		stats[c].degSum += g.Strength(nid)
		for _, e := range g.Neighbors(nid) {
			neighborComm, ok2 := partition[e.To]
			if !ok2 {
				neighborComm = -1
			}
			if neighborComm == c {
				stats[c].intraWeight += e.Weight
			}
		}
	}

	q := 0.0
	for _, s := range stats {
		q += s.intraWeight/twoW - resolution*(s.degSum/twoW)*(s.degSum/twoW)
	}
	return q
}
