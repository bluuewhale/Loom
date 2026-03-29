package graph

import (
	"math/rand"
	"time"
)

// leidenState holds mutable state for a single Leiden detection run.
// It mirrors louvainState but adds refinedPartition for the BFS refinement phase.
type leidenState struct {
	partition        map[NodeID]int  // node -> community ID (from local-move phase)
	refinedPartition map[NodeID]int  // after BFS split; used for supergraph aggregation
	commStr          map[int]float64 // community ID -> sum of node strengths (cached)
	rng              *rand.Rand      // per-run RNG for node shuffle
}

// newLeidenState initializes state with each node in its own community.
// Community IDs start at 0 and are assigned in ascending NodeID order for
// deterministic initialization regardless of map iteration order.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
func newLeidenState(g *Graph, seed int64) *leidenState {
	var src rand.Source
	if seed != 0 {
		src = rand.NewSource(seed)
	} else {
		src = rand.NewSource(time.Now().UnixNano())
	}

	nodes := g.Nodes()
	// Sort nodes by ID for deterministic community ID assignment.
	for i := 1; i < len(nodes); i++ {
		for j := i; j > 0 && nodes[j] < nodes[j-1]; j-- {
			nodes[j], nodes[j-1] = nodes[j-1], nodes[j]
		}
	}

	partition := make(map[NodeID]int, len(nodes))
	commStr := make(map[int]float64, len(nodes))

	for i, n := range nodes {
		partition[n] = i
		commStr[i] = g.Strength(n)
	}

	return &leidenState{
		partition:        partition,
		refinedPartition: nil, // populated after first refinement
		commStr:          commStr,
		rng:              rand.New(src),
	}
}
