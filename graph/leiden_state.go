package graph

import (
	"math/rand/v2"
	"sync"
	"time"
)

// leidenState holds mutable state for a single Leiden detection run.
// It mirrors louvainState but adds refinedPartition for the BFS refinement phase.
type leidenState struct {
	partition        map[NodeID]int // node -> community ID (from local-move phase)
	refinedPartition map[NodeID]int // after BFS split; used for supergraph aggregation
	commStr          []float64      // community ID -> sum of node strengths; indexed by comm ID (always in [0,n))
	neighborBuf      []float64      // comm ID -> accumulated edge weight from current node; indexed by comm ID
	neighborSeen     []bool         // comm ID -> whether comm has been added to candidateBuf this iteration
	neighborDirty    []int          // comm IDs written to neighborBuf/neighborSeen this iteration (for reset)
	candidateBuf     []int          // reusable buffer for candidate communities
	idxBuf           []int32        // reusable buffer for dense-index shuffle in phase1
	partitionByIdx   []int          // CSR dense-index -> community ID; built once per phase1 call to eliminate map lookups
	rng              *rand.Rand     // per-run RNG for node shuffle
	pcg              *rand.PCG      // stored source for zero-alloc reseed

	// refinePartitionInPlace scratch — eliminates all per-community allocations:
	commBuildPairs []commNodePair // (comm, node) pairs; grown lazily
	inCommBits     []bool         // CSR-indexed; true if node is in current community
	visitedBits    []bool         // CSR-indexed; true if node has been BFS-visited

	// counting-sort scratch — replaces slices.SortFunc for O(N) community grouping:
	commCountScratch []int          // indexed by partition ID (always in [0,N)); reset via commSeenComms
	commSeenComms    []int          // dirty list of community IDs touched; used to zero commCountScratch
	commSortedPairs  []commNodePair // scatter output buffer; same size as commBuildPairs

	// commToNewSuperBuf is a reusable slice for comm ID -> new supernode NodeID mapping.
	// Avoids allocating a new map each supergraph iteration in runOnce.
	commToNewSuperBuf []NodeID

	// sgScratch holds reusable buffers for buildSupergraph calls.
	sgScratch supergraphScratch
	// csrBuf holds a reusable CSR view; filled in-place by buildCSRInto each pass.
	csrBuf csrGraph

	// nodeMappingSliceA/B replace the old map-based nodeMapping: position i holds the
	// current supernode NodeID for origNodes[i]. Slice copy/range outperforms map copy/range.
	nodeMappingSliceA []NodeID
	nodeMappingSliceB []NodeID
	bestSuperPartBuf  map[NodeID]int
	// normUsedBuf and normRemapBuf are reusable scratch for normalizePartition.
	normUsedBuf  []bool
	normRemapBuf []int

	// BFS queue of CSR dense indices (int32) — avoids g.Neighbors map lookup:
	bfsQueue []int32

	// scratchPartition is a reusable map[NodeID]int for per-pass candidateQ computation
	// in runOnce. Avoids allocating a new N-entry map on every supergraph iteration.
	scratchPartition map[NodeID]int
}

// leidenStatePool reuses leidenState allocations across Detect calls to reduce GC pressure.
var leidenStatePool = sync.Pool{
	New: func() any {
		pcg := rand.NewPCG(1, 0)
		return &leidenState{
			// Pre-sized to typical ego-net node count (≤128) so fresh pool entries
			// never trigger map-bucket growth or slice realloc on first use.
			partition:        make(map[NodeID]int, 128),
			refinedPartition: make(map[NodeID]int, 128),
			commStr:          make([]float64, 128),
			neighborBuf:      make([]float64, 128),
			neighborSeen:     make([]bool, 128),
			neighborDirty:    make([]int, 0, 64),
			candidateBuf:     make([]int, 0, 64),
			idxBuf:           make([]int32, 0, 128),
			partitionByIdx:   make([]int, 128),
			rng:              rand.New(pcg),
			pcg:              pcg,
			commBuildPairs:   make([]commNodePair, 0, 128),
			commSeenComms:    make([]int, 0, 64),
			commSortedPairs:  make([]commNodePair, 0, 128),
			bfsQueue:         make([]int32, 0, 64),
			scratchPartition: make(map[NodeID]int, 128),
			sgScratch:         supergraphScratch{interEdges: make(map[edgeKey]float64, 64)},
			nodeMappingSliceA: make([]NodeID, 0, 128),
			nodeMappingSliceB: make([]NodeID, 0, 128),
			bestSuperPartBuf:  make(map[NodeID]int, 128),
			// inCommBits, visitedBits, commCountScratch grown lazily in refinePartitionInPlace.
		}
	},
}

// acquireLeidenState obtains a leidenState from the pool.
// The caller is responsible for calling st.reset before first use;
// runOnce does this via its firstPass branch.
func acquireLeidenState(_ *Graph, _ int64) *leidenState {
	return leidenStatePool.Get().(*leidenState)
}

// releaseLeidenState returns st to the pool. Callers must not use st after this call.
func releaseLeidenState(st *leidenState) {
	leidenStatePool.Put(st)
}

// reset reinitializes st for a new Detect pass on g.
// Clears and repopulates all maps/slices; resets slice lengths without freeing capacity.
// Seed 0 uses time.Now().UnixNano() (non-deterministic).
// initialPartition nil = cold start (existing behavior); non-nil = warm start.
// refinedPartition is always cleared; it is repopulated after BFS refinement.
func (st *leidenState) reset(g *Graph, seed int64, initialPartition map[NodeID]int) {
	// Clear existing map contents (reuse allocated maps).
	clear(st.partition)
	clear(st.refinedPartition)

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
