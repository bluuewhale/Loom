package graph

import (
	"math/rand/v2"
	"sync"
	"time"
)

// louvainState holds mutable state for a single Louvain detection run.
type louvainState struct {
	partition     map[NodeID]int     // node -> community ID
	commStr       map[int]float64    // community ID -> sum of node strengths (cached)
	neighborBuf   map[NodeID]float64 // reusable buffer: neighbor community weight accumulation
	neighborDirty []NodeID           // dirty-list: keys written to neighborBuf this iteration
	candidateBuf  []int              // reusable buffer for candidate community IDs
	idxBuf        []int32            // reusable buffer for dense-index shuffle in phase1
	rng           *rand.Rand         // per-run RNG for node shuffle
	pcg           *rand.PCG          // stored source for zero-alloc reseed
}

// louvainStatePool reuses louvainState allocations across Detect calls to reduce GC pressure.
var louvainStatePool = sync.Pool{
	New: func() any {
		pcg := rand.NewPCG(1, 0)
		return &louvainState{
			partition:     make(map[NodeID]int),
			commStr:       make(map[int]float64),
			neighborBuf:   make(map[NodeID]float64),
			neighborDirty: make([]NodeID, 0, 64),
			candidateBuf:  make([]int, 0, 64),
			idxBuf:        make([]int32, 0, 128),
			rng:           rand.New(pcg),
			pcg:           pcg,
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
	st.idxBuf = st.idxBuf[:0]

	// Re-seed RNG via stored PCG source — zero allocation.
	var actualSeed int64
	if seed != 0 {
		actualSeed = seed
	} else {
		actualSeed = time.Now().UnixNano()
	}
	if st.pcg == nil {
		// First use (not from pool) — allocate once.
		st.pcg = rand.NewPCG(uint64(actualSeed), 0)
		st.rng = rand.New(st.pcg)
	} else {
		st.pcg.Seed(uint64(actualSeed), 0)
	}

	// Populate communities in ascending NodeID order for determinism.
	nodes := g.Nodes() // cached, already sorted

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

