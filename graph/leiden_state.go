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
	sortedNodes      []NodeID           // cached sorted node list; reused when node set unchanged
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
	// Save previous commStr before clearing — used by warm-start delta patch (Step 4).
	var prevCommStr map[int]float64
	if initialPartition != nil && len(st.commStr) > 0 {
		prevCommStr = make(map[int]float64, len(st.commStr))
		for k, v := range st.commStr {
			prevCommStr[k] = v
		}
	}

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

	// Populate communities in ascending NodeID order for determinism.
	// sortedNodes cache: reuse cached slice when node set is unchanged (O(1) vs O(N log N)).
	nodeCount := g.NodeCount()
	var nodes []NodeID
	if len(st.sortedNodes) == nodeCount {
		// Warm-start with unchanged node set — reuse cached sorted slice.
		nodes = st.sortedNodes
	} else {
		// Node set changed (or cold first use) — fetch, sort, and cache.
		nodes = g.Nodes()
		slices.Sort(nodes)
		st.sortedNodes = nodes
	}

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

	// Step 4: commStr delta patch — O(|communities|) + O(|new_nodes|) instead of O(N).
	//
	// The delta patch is only valid when prevCommStr's key set exactly matches the set
	// of community IDs in initialPartition. If they differ (e.g. pool state was reused
	// by a different Detect call), fall back to the O(N) full rebuild.
	//
	// Compatibility check: collect unique community IDs from initialPartition and compare
	// against prevCommStr key set.
	deltaValid := false
	if prevCommStr != nil {
		initCommSet := make(map[int]struct{}, len(prevCommStr))
		for _, c := range initialPartition {
			initCommSet[c] = struct{}{}
		}
		if len(initCommSet) == len(prevCommStr) {
			deltaValid = true
			for c := range initCommSet {
				if _, ok := prevCommStr[c]; !ok {
					deltaValid = false
					break
				}
			}
		}
	}

	if deltaValid {
		// 4a: Remap previous community strengths to new compact IDs.
		for oldC, str := range prevCommStr {
			if newC, ok := remap[oldC]; ok {
				st.commStr[newC] = str
			}
		}
		// 4b: Patch new nodes (not in initialPartition) — add their strength.
		for _, n := range nodes {
			if _, ok := initialPartition[n]; !ok {
				st.commStr[st.partition[n]] += g.Strength(n)
			}
		}
	} else {
		// Fallback: full O(N) rebuild when prevCommStr doesn't match initialPartition.
		for _, n := range nodes {
			st.commStr[st.partition[n]] += g.Strength(n)
		}
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
