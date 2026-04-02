package graph

import (
	"math/rand/v2"
	"sync"
	"time"
)

// louvainState holds mutable state for a single Louvain detection run.
type louvainState struct {
	partition      map[NodeID]int // node -> community ID
	commStr        []float64      // community ID -> sum of node strengths; indexed by comm ID (always in [0,n))
	neighborBuf    []float64      // comm ID -> accumulated edge weight from current node; indexed by comm ID
	neighborSeen   []bool         // comm ID -> whether comm has been added to candidateBuf this iteration
	neighborDirty  []int          // comm IDs written to neighborBuf/neighborSeen this iteration (for reset)
	candidateBuf   []int          // reusable buffer for candidate community IDs
	idxBuf            []int32           // reusable buffer for dense-index shuffle in phase1
	partitionByIdx    []int             // CSR dense-index -> community ID; built once per phase1 call to eliminate map lookups
	commToNewSuperBuf []NodeID          // reusable slice for comm ID -> new supernode NodeID mapping; avoids per-pass map alloc
	sgScratch         supergraphScratch // reusable buffers for buildSupergraph; eliminates ~12 allocs/call
	csrBuf            csrGraph          // reusable CSR view; filled in-place by buildCSRInto each pass
	// nodeMappingSliceA/B replace the old map-based nodeMapping: position i holds the
	// current supernode NodeID for origNodes[i]. Slice copy/range outperforms map copy/range
	// for the small ego-nets that dominate the hot path.
	nodeMappingSliceA []NodeID
	nodeMappingSliceB []NodeID
	bestSuperPartBuf  map[NodeID]int
	// normUsedBuf and normRemapBuf are reusable scratch for normalizePartition.
	normUsedBuf  []bool
	normRemapBuf []int
	// scratchPartition is a reusable map for per-pass candidateQ computation in runOnce.
	// Avoids allocating a new N-entry map on every supergraph pass.
	scratchPartition map[NodeID]int
	rng              *rand.Rand // per-run RNG for node shuffle
	pcg              *rand.PCG  // stored source for zero-alloc reseed
}

// louvainStatePool reuses louvainState allocations across Detect calls to reduce GC pressure.
var louvainStatePool = sync.Pool{
	New: func() any {
		pcg := rand.NewPCG(1, 0)
		return &louvainState{
			// Pre-sized to the typical ego-net node count (≤128) so that fresh pool
			// entries never trigger map-bucket growth or slice reallocation on first use.
			// The growth guards in reset() still handle graphs larger than 128.
			partition:      make(map[NodeID]int, 128),
			commStr:        make([]float64, 128),
			neighborBuf:    make([]float64, 128),
			neighborSeen:   make([]bool, 128),
			neighborDirty:  make([]int, 0, 64),
			candidateBuf:   make([]int, 0, 64),
			idxBuf:         make([]int32, 0, 128),
			partitionByIdx: make([]int, 128),
			sgScratch:         supergraphScratch{interEdges: make(map[edgeKey]float64, 64)},
			nodeMappingSliceA: make([]NodeID, 0, 128),
			nodeMappingSliceB: make([]NodeID, 0, 128),
			bestSuperPartBuf:  make(map[NodeID]int, 128),
			scratchPartition: make(map[NodeID]int, 128),
			rng:              rand.New(pcg),
			pcg:              pcg,
		}
	},
}

// acquireLouvainState obtains a louvainState from the pool.
// The caller is responsible for calling st.reset before first use;
// runOnce does this via its firstPass branch.
func acquireLouvainState(_ *Graph, _ int64) *louvainState {
	return louvainStatePool.Get().(*louvainState)
}

// releaseLouvainState returns st to the pool. Callers must not use st after this call.
func releaseLouvainState(st *louvainState) {
	louvainStatePool.Put(st)
}

// reset reinitializes st for a new Detect pass on g.
// Clears and repopulates all maps/slices; resets slice lengths without freeing capacity.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
// initialPartition nil = cold start (existing behavior); non-nil = warm start.
func (st *louvainState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
	// Clear existing partition map (reuse allocated map).
	clear(st.partition)

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
	sz := len(nodes)

	// Grow flat arrays if needed (new allocations are zero-initialized).
	// Community IDs are always in [0, sz) — see invariant comment in refinePartitionInPlace.
	if len(st.commStr) < sz {
		st.commStr = make([]float64, sz)
		st.neighborBuf = make([]float64, sz)
		st.neighborSeen = make([]bool, sz)
		st.neighborDirty = st.neighborDirty[:0] // new arrays are already zero
	} else {
		// Clear neighborBuf/neighborSeen via dirty list (only touched entries).
		for _, c := range st.neighborDirty {
			st.neighborBuf[c] = 0.0
			st.neighborSeen[c] = false
		}
		st.neighborDirty = st.neighborDirty[:0]
	}
	st.candidateBuf = st.candidateBuf[:0]
	st.idxBuf = st.idxBuf[:0]

	// Grow partitionByIdx to match CSR dense-index count (== sz).
	// phase1 populates it from st.partition at the start of each call.
	if len(st.partitionByIdx) < sz {
		st.partitionByIdx = make([]int, sz)
	} else {
		st.partitionByIdx = st.partitionByIdx[:sz]
	}

	if initialPartition == nil {
		// Cold start: trivial singleton assignment.
		// Directly overwrite commStr entries — no need to zero first.
		for i, nid := range nodes {
			st.partition[nid] = i
			st.commStr[i] = g.Strength(nid)
		}
		return
	}

	// Warm start: seed from initialPartition.
	// commStr will be built by accumulation — zero [0, sz) first.
	clear(st.commStr[:sz])

	// Step 1: find max community ID for new-node singleton offset.
	maxCommID := -1
	for _, c := range initialPartition {
		if c > maxCommID {
			maxCommID = c
		}
	}
	nextNewComm := maxCommID + 1

	// Step 2: assign partition; new nodes not in prior partition get fresh singletons.
	for _, nid := range nodes {
		if c, ok := initialPartition[nid]; ok {
			st.partition[nid] = c
		} else {
			st.partition[nid] = nextNewComm
			nextNewComm++
		}
	}

	// Step 3: compact to 0-indexed contiguous IDs (nodes already sorted — deterministic remap).
	// Use normRemapBuf (a pooled []int) instead of map[int]int to eliminate one heap alloc.
	// normRemapBuf is overwritten by normalizePartitionWithBufs at the end of runOnce, so
	// borrowing it here is safe (the two uses do not overlap).
	maxPartComm := 0
	for _, c := range st.partition {
		if c > maxPartComm {
			maxPartComm = c
		}
	}
	needRemap := maxPartComm + 1
	if cap(st.normRemapBuf) >= needRemap {
		st.normRemapBuf = st.normRemapBuf[:needRemap]
	} else {
		st.normRemapBuf = make([]int, needRemap)
	}
	for i := range st.normRemapBuf {
		st.normRemapBuf[i] = -1
	}
	remap := st.normRemapBuf
	next := 0
	for _, nid := range nodes {
		c := st.partition[nid]
		if remap[c] == -1 {
			remap[c] = next
			next++
		}
		st.partition[nid] = remap[c]
	}

	// Step 4: build commStr from CURRENT graph strengths (not from prior run).
	for _, nid := range nodes {
		st.commStr[st.partition[nid]] += g.Strength(nid)
	}
}
