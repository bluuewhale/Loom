package graph

import (
	"math/rand"
	"slices"
	"sync"
	"time"
)

// louvainState holds mutable state for a single Louvain detection run.
type louvainState struct {
	partition     map[NodeID]int  // node -> community ID
	commStr       map[int]float64 // community ID -> sum of node strengths (cached)
	neighborBuf   map[NodeID]float64 // reusable buffer: neighbor community weight accumulation
	neighborDirty []NodeID           // dirty-list: keys written to neighborBuf this iteration
	candidateBuf  []int              // reusable buffer for candidate community IDs
	rng           *rand.Rand         // per-run RNG for node shuffle
}

// louvainStatePool reuses louvainState allocations across Detect calls to reduce GC pressure.
var louvainStatePool = sync.Pool{
	New: func() any {
		return &louvainState{
			partition:     make(map[NodeID]int),
			commStr:       make(map[int]float64),
			neighborBuf:   make(map[NodeID]float64),
			neighborDirty: make([]NodeID, 0, 64),
			candidateBuf:  make([]int, 0, 64),
		}
	},
}

// acquireLouvainState obtains a louvainState from the pool and resets it for g.
func acquireLouvainState(g *Graph, seed int64) *louvainState {
	st := louvainStatePool.Get().(*louvainState)
	st.reset(g, seed, nil)
	return st
}

// releaseLouvainState returns st to the pool. Callers must not use st after this call.
func releaseLouvainState(st *louvainState) {
	louvainStatePool.Put(st)
}

// reset reinitializes st for a new Detect pass on g.
// Clears and repopulates all maps; resets slice lengths without freeing capacity.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
// initialPartition nil = cold start (existing behavior); non-nil = warm start.
func (st *louvainState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
	// Clear existing map contents (reuse allocated map).
	clear(st.partition)
	clear(st.commStr)
	clear(st.neighborBuf)
	st.neighborDirty = st.neighborDirty[:0]
	st.candidateBuf = st.candidateBuf[:0]

	// Re-seed RNG. Always create a fresh rand.New to ensure identical number
	// generation to newLouvainState; st.rng.Seed skips internal state setup.
	var src rand.Source
	if seed != 0 {
		src = rand.NewSource(seed)
	} else {
		src = rand.NewSource(time.Now().UnixNano())
	}
	st.rng = rand.New(src)

	// Populate communities in ascending NodeID order for determinism.
	nodes := g.Nodes()
	slices.Sort(nodes)

	if initialPartition == nil {
		// Cold start: trivial singleton assignment (existing behavior).
		for i, n := range nodes {
			st.partition[n] = i
			st.commStr[i] = g.Strength(n)
		}
		return
	}

	// Warm start: seed from initialPartition.
	// Step 1: find max community ID for new-node singleton offset.
	maxCommID := -1
	for _, c := range initialPartition {
		if c > maxCommID {
			maxCommID = c
		}
	}
	nextNewComm := maxCommID + 1

	// Step 2: assign partition; new nodes not in prior partition get fresh singletons.
	for _, n := range nodes {
		if c, ok := initialPartition[n]; ok {
			st.partition[n] = c
		} else {
			st.partition[n] = nextNewComm
			nextNewComm++
		}
	}

	// Step 3: compact to 0-indexed contiguous IDs (nodes already sorted — deterministic remap).
	remap := make(map[int]int, len(nodes))
	next := 0
	for _, n := range nodes {
		c := st.partition[n]
		if _, exists := remap[c]; !exists {
			remap[c] = next
			next++
		}
		st.partition[n] = remap[c]
	}

	// Step 4: build commStr from CURRENT graph strengths (not from prior run).
	for _, n := range nodes {
		st.commStr[st.partition[n]] += g.Strength(n)
	}
}

// newLouvainState is kept for backward-compatibility with code that creates
// a louvainState without using the pool (e.g., the Leiden inline wrapper).
func newLouvainState(g *Graph, seed int64) *louvainState {
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

	return &louvainState{
		partition:     partition,
		commStr:       commStr,
		neighborBuf:   make(map[NodeID]float64),
		neighborDirty: make([]NodeID, 0, 64),
		candidateBuf:  make([]int, 0, 64),
		rng:           rand.New(src),
	}
}
