package graph

import (
	"math/rand"
	"slices"
	"sync"
	"time"
)

// leidenState holds mutable state for a single Leiden detection run.
// It mirrors louvainState but adds refinedPartition for the BFS refinement phase.
type leidenState struct {
	partition        map[NodeID]int     // node -> community ID (from local-move phase)
	refinedPartition map[NodeID]int     // after BFS split; used for supergraph aggregation
	commStr          map[int]float64    // community ID -> sum of node strengths (cached)
	neighborBuf      map[NodeID]float64 // reusable buffer for neighbor weight accumulation
	neighborDirty    []NodeID           // dirty-list: keys written to neighborBuf this iteration
	candidateBuf     []int              // reusable buffer for candidate communities
	rng              *rand.Rand         // per-run RNG for node shuffle
}

// leidenStatePool reuses leidenState allocations across Detect calls to reduce GC pressure.
var leidenStatePool = sync.Pool{
	New: func() any {
		return &leidenState{
			partition:        make(map[NodeID]int),
			refinedPartition: make(map[NodeID]int),
			commStr:          make(map[int]float64),
			neighborBuf:      make(map[NodeID]float64),
			neighborDirty:    make([]NodeID, 0, 64),
			candidateBuf:     make([]int, 0, 64),
		}
	},
}

// acquireLeidenState obtains a leidenState from the pool and resets it for g.
func acquireLeidenState(g *Graph, seed int64) *leidenState {
	st := leidenStatePool.Get().(*leidenState)
	st.reset(g, seed)
	return st
}

// releaseLeidenState returns st to the pool. Callers must not use st after this call.
func releaseLeidenState(st *leidenState) {
	leidenStatePool.Put(st)
}

// reset reinitializes st for a new Detect pass on g.
// Clears and repopulates all maps; resets slice lengths without freeing capacity.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
func (st *leidenState) reset(g *Graph, seed int64) {
	// Clear existing map contents (reuse allocated map).
	clear(st.partition)
	clear(st.refinedPartition)
	clear(st.commStr)
	clear(st.neighborBuf)
	st.neighborDirty = st.neighborDirty[:0]
	st.candidateBuf = st.candidateBuf[:0]

	// Re-seed RNG. Always create a fresh rand.New to ensure identical number
	// generation to newLeidenState; st.rng.Seed skips internal state setup.
	var src rand.Source
	if seed != 0 {
		src = rand.NewSource(seed)
	} else {
		src = rand.NewSource(time.Now().UnixNano())
	}
	st.rng = rand.New(src)

	// Populate singleton communities in ascending NodeID order for determinism.
	nodes := g.Nodes()
	slices.Sort(nodes)
	for i, n := range nodes {
		st.partition[n] = i
		st.commStr[i] = g.Strength(n)
	}
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
	slices.Sort(nodes)

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
		neighborBuf:      make(map[NodeID]float64),
		neighborDirty:    make([]NodeID, 0, 64),
		candidateBuf:     make([]int, 0, 64),
		rng:              rand.New(src),
	}
}
