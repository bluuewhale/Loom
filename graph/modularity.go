package graph

import "sync"

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

// modBufs holds reusable flat float64 accumulators for ComputeModularityWeighted.
type modBufs struct {
	intra []float64 // indexed by community ID
	deg   []float64 // indexed by community ID
}

var modBufsPool = sync.Pool{
	New: func() any {
		return &modBufs{
			intra: make([]float64, 128),
			deg:   make([]float64, 128),
		}
	},
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

	// Find max community ID to size flat accumulators.
	// Scan via g.Nodes() (cached sorted slice) to avoid maps.(*Iter).Next overhead;
	// partition is keyed by NodeID so this is equivalent to ranging over partition
	// for any partition whose domain is a subset of g's nodes.
	maxComm := -1
	for _, nid := range g.Nodes() {
		if c, ok := partition[nid]; ok && c > maxComm {
			maxComm = c
		}
	}
	if maxComm < 0 {
		return 0.0
	}
	sz := maxComm + 1

	// Acquire pooled buffers; grow if needed, then zero to exact size.
	bufs := modBufsPool.Get().(*modBufs)
	if cap(bufs.intra) < sz {
		bufs.intra = make([]float64, sz)
		bufs.deg = make([]float64, sz)
	} else {
		bufs.intra = bufs.intra[:sz]
		bufs.deg = bufs.deg[:sz]
		clear(bufs.intra)
		clear(bufs.deg)
	}
	defer modBufsPool.Put(bufs)

	// Accumulate intra-community edge weights and degree sums.
	// Each neighbor loop is a single pass over adjacency (computes strength inline).
	// Nodes missing from partition are grouped into a virtual community -1.
	var missingIntra, missingDeg float64
	for _, nid := range g.Nodes() {
		c, ok := partition[nid]
		if !ok {
			// Virtual community -1: count strength and intra-edges among missing nodes.
			for _, e := range g.adjacency[nid] {
				missingDeg += e.Weight
				if _, ok2 := partition[e.To]; !ok2 {
					missingIntra += e.Weight
				}
			}
			continue
		}
		for _, e := range g.adjacency[nid] {
			bufs.deg[c] += e.Weight
			if nc, ok2 := partition[e.To]; ok2 && nc == c {
				bufs.intra[c] += e.Weight
			}
		}
	}

	q := 0.0
	// Contribution from virtual community -1 (missing nodes).
	if missingDeg > 0 || missingIntra > 0 {
		q += missingIntra/twoW - resolution*(missingDeg/twoW)*(missingDeg/twoW)
	}
	for c := 0; c < sz; c++ {
		if bufs.deg[c] > 0 || bufs.intra[c] > 0 {
			q += bufs.intra[c]/twoW - resolution*(bufs.deg[c]/twoW)*(bufs.deg[c]/twoW)
		}
	}
	return q
}
