package graph

import (
	"math/rand/v2"
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
	pcg              *rand.PCG          // stored source for zero-alloc reseed

	// refinePartitionInPlace scratch — eliminates all per-community allocations:
	commBuildPairs []commNodePair // (comm, node) pairs; grown lazily
	inCommBits     []bool         // CSR-indexed; true if node is in current community
	visitedBits    []bool         // CSR-indexed; true if node has been BFS-visited

	// counting-sort scratch — replaces slices.SortFunc for O(N) community grouping:
	commCountScratch []int // indexed by partition ID (always in [0,N)); reset via commSeenComms
	commSeenComms    []int // dirty list of community IDs touched; used to zero commCountScratch
	commSortedPairs  []commNodePair // scatter output buffer; same size as commBuildPairs

	// BFS queue of CSR dense indices (int32) — avoids g.Neighbors map lookup:
	bfsQueue []int32
}

// leidenStatePool reuses leidenState allocations across Detect calls to reduce GC pressure.
var leidenStatePool = sync.Pool{
	New: func() any {
		pcg := rand.NewPCG(1, 0)
		return &leidenState{
			partition:        make(map[NodeID]int),
			refinedPartition: make(map[NodeID]int),
			commStr:          make(map[int]float64),
			neighborBuf:      make(map[NodeID]float64),
			neighborDirty:    make([]NodeID, 0, 64),
			candidateBuf:     make([]int, 0, 64),
			rng:              rand.New(pcg),
			pcg:              pcg,
			commBuildPairs:  make([]commNodePair, 0, 128),
			commSeenComms:   make([]int, 0, 64),
			commSortedPairs: make([]commNodePair, 0, 128),
			bfsQueue:        make([]int32, 0, 64),
			// inCommBits, visitedBits, commCountScratch grown lazily in refinePartitionInPlace.
		}
	},
}

// acquireLeidenState obtains a leidenState from the pool and resets it for g.
func acquireLeidenState(g *Graph, seed int64) *leidenState {
	st := leidenStatePool.Get().(*leidenState)
	st.reset(g, seed, nil)
	return st
}

// releaseLeidenState returns st to the pool. Callers must not use st after this call.
func releaseLeidenState(st *leidenState) {
	leidenStatePool.Put(st)
}

// reset reinitializes st for a new Detect pass on g.
// Clears and repopulates all maps; resets slice lengths without freeing capacity.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
// initialPartition nil = cold start (existing behavior); non-nil = warm start.
// refinedPartition is always cleared; it is repopulated after BFS refinement.
func (st *leidenState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
	// Clear existing map contents (reuse allocated map).
	clear(st.partition)
	clear(st.refinedPartition)
	clear(st.commStr)
	clear(st.neighborBuf)
	st.neighborDirty = st.neighborDirty[:0]
	st.candidateBuf = st.candidateBuf[:0]

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

