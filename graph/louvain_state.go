package graph

import (
	"math/rand"
	"time"
)

// louvainState holds mutable state for a single Louvain detection run.
type louvainState struct {
	partition map[NodeID]int  // node -> community ID
	commStr   map[int]float64 // community ID -> sum of node strengths (cached)
	rng       *rand.Rand      // per-run RNG for node shuffle
}

// newLouvainState initializes state with each node in its own community.
// Community IDs start at 0 and are assigned in ascending NodeID order for
// deterministic initialization regardless of map iteration order.
func newLouvainState(g *Graph, seed int64) *louvainState {
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

	return &louvainState{
		partition: partition,
		commStr:   commStr,
		rng:       rand.New(src),
	}
}
