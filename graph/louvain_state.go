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
// Community IDs start at 0 and are assigned in Nodes() order.
func newLouvainState(g *Graph, seed int64) *louvainState {
	var src rand.Source
	if seed != 0 {
		src = rand.NewSource(seed)
	} else {
		src = rand.NewSource(time.Now().UnixNano())
	}

	nodes := g.Nodes()
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
