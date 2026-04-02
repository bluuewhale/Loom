package graph

import (
	"fmt"
	"math"
	"slices"
	"time"
)

// commNodePair pairs a community ID with a NodeID for the sorted-partition BFS approach.
type commNodePair struct {
	comm int
	node NodeID
}

// Detect runs the Leiden community detection algorithm on graph g.
// Leiden improves on Louvain by guaranteeing internally-connected communities:
// after each local-move phase, a BFS refinement splits any disconnected
// community into its connected components before supergraph aggregation.
//
// It returns ErrDirectedNotSupported for directed graphs.
// For empty graphs, it returns an empty CommunityResult with no error.
// The returned Partition is always 0-indexed contiguous.
//
// When Seed != 0, a single deterministic run is performed (NumRuns is ignored).
// When Seed == 0 and NumRuns > 1 (or NumRuns == 0, defaulting to 3), multiple
// independent runs are executed and the best-Q result is returned.
func (d *leidenDetector) Detect(g *Graph) (CommunityResult, error) {
	// --- Guard clauses ---
	if g.IsDirected() {
		return CommunityResult{}, ErrDirectedNotSupported
	}
	if g.NodeCount() == 0 {
		return CommunityResult{}, nil
	}
	if g.NodeCount() == 1 {
		node := g.Nodes()[0]
		return CommunityResult{
			Partition:  map[NodeID]int{node: 0},
			Modularity: 0.0,
			Passes:     1,
			Moves:      0,
		}, nil
	}
	if g.TotalWeight() == 0 {
		// All nodes disconnected: each in own community.
		nodes := g.Nodes()
		p := make(map[NodeID]int, len(nodes))
		for i, n := range nodes {
			p[n] = i
		}
		return CommunityResult{
			Partition:  p,
			Modularity: 0.0,
			Passes:     1,
			Moves:      0,
		}, nil
	}

	// Seed!=0 → single deterministic run; NumRuns is ignored entirely.
	if d.opts.Seed != 0 {
		return d.runOnce(g, d.opts.Seed)
	}

	// Seed==0: resolve NumRuns (0 → default 3).
	effectiveNumRuns := d.opts.NumRuns
	if effectiveNumRuns == 0 {
		effectiveNumRuns = 3
	}
	// NumRuns==1 → single run with a random seed.
	if effectiveNumRuns == 1 {
		return d.runOnce(g, 0)
	}

	// Multi-run: compute baseSeed once before the loop (avoids same-nanosecond collisions).
	baseSeed := time.Now().UnixNano()
	var bestResult CommunityResult
	bestQ := math.Inf(-1)
	var lastErr error
	for i := 0; i < effectiveNumRuns; i++ {
		res, err := d.runOnce(g, baseSeed+int64(i))
		if err != nil {
			lastErr = err
			continue
		}
		if res.Modularity > bestQ {
			bestQ = res.Modularity
			bestResult = res
		}
	}
	if bestQ == math.Inf(-1) {
		return CommunityResult{}, lastErr
	}
	// At least one run succeeded — return its result. We intentionally discard lastErr
	// because partial multi-run failures (some iterations succeed, others fail) should
	// not prevent returning the best successful result.
	return bestResult, nil
}

// runOnce executes a single Leiden detection run on g using the given seed.
// seed==0 triggers non-deterministic (time-based) seeding inside acquireLeidenState.
// The returned Partition is always 0-indexed contiguous.
func (d *leidenDetector) runOnce(g *Graph, seed int64) (CommunityResult, error) {
	// --- Resolve zero-value options ---
	resolution := d.opts.Resolution
	if resolution == 0.0 {
		resolution = 1.0
	}
	maxIterations := d.opts.MaxIterations // 0 = unlimited

	// Acquire pooled state early so we can reuse its map buffers for nodeMapping too.
	state := acquireLeidenState(g, seed)
	defer releaseLeidenState(state)

	// nodeMappingSlice tracks origNodes[i] → current supernode NodeID.
	origNodes := g.Nodes()
	sz := len(origNodes)
	if cap(state.nodeMappingSliceA) < sz {
		state.nodeMappingSliceA = make([]NodeID, sz, sz+sz/4+1)
	} else {
		state.nodeMappingSliceA = state.nodeMappingSliceA[:sz]
	}
	copy(state.nodeMappingSliceA, origNodes) // identity mapping
	nodeMappingSlice := state.nodeMappingSliceA

	currentGraph := g
	buildCSRInto(currentGraph, &state.csrBuf)
	csr := state.csrBuf // cheap header copy; backing arrays shared with state.csrBuf
	totalPasses := 0
	totalMoves := 0

	// Best-partition tracking: retain the highest-Q partition found so far.
	bestQ := math.Inf(-1)
	if cap(state.nodeMappingSliceB) < sz {
		state.nodeMappingSliceB = make([]NodeID, sz, sz+sz/4+1)
	} else {
		state.nodeMappingSliceB = state.nodeMappingSliceB[:sz]
	}
	copy(state.nodeMappingSliceB, origNodes) // identity mapping
	bestNodeMappingSlice := state.nodeMappingSliceB
	bestSuperPartition := state.bestSuperPartBuf
	clear(bestSuperPartition)

	firstPass := true
	for {
		if firstPass {
			state.reset(currentGraph, seed, d.opts.InitialPartition)
			firstPass = false
		} else {
			state.reset(currentGraph, seed, nil)
		}

		// Phase 1: local move — reuse Louvain phase1 via louvainState wrapper.
		// neighborBuf/neighborSeen/commStr are flat slices shared by pointer; no copy-back needed.
		// neighborDirty/candidateBuf/idxBuf may be appended beyond capacity — copy back headers.
		ls := &louvainState{
			partition:      state.partition,
			commStr:        state.commStr,
			neighborBuf:    state.neighborBuf,
			neighborSeen:   state.neighborSeen,
			neighborDirty:  state.neighborDirty,
			candidateBuf:   state.candidateBuf,
			idxBuf:         state.idxBuf,
			partitionByIdx: state.partitionByIdx,
			rng:            state.rng,
		}
		moves := phase1(currentGraph, &csr, ls, resolution, currentGraph.TotalWeight())
		state.partition = ls.partition
		state.neighborDirty = ls.neighborDirty
		state.candidateBuf = ls.candidateBuf
		state.idxBuf = ls.idxBuf
		state.partitionByIdx = ls.partitionByIdx
		totalPasses++
		totalMoves += moves

		// Phase 2 (Leiden-specific): BFS refinement — split disconnected communities.
		refinePartitionInPlace(currentGraph, &csr, state.partition, state)

		// Best-Q tracking using refinedPartition (reflects actual aggregation structure).
		// Reuse scratchPartition to avoid allocating a new N-entry map every pass.
		reconstructPartitionIntoSlice(origNodes, nodeMappingSlice, state.refinedPartition, state.scratchPartition)
		candidateQ := ComputeModularityWeighted(g, state.scratchPartition, resolution)
		if candidateQ > bestQ {
			bestQ = candidateQ
			copy(bestNodeMappingSlice, nodeMappingSlice) // O(N) slice copy; no map overhead
			// Copy refinedPartition → bestSuperPartition (in-place, pooled map).
			clear(bestSuperPartition)
			for k, v := range state.refinedPartition {
				bestSuperPartition[k] = v
			}
		}

		if moves == 0 {
			break
		}
		if maxIterations > 0 && totalPasses >= maxIterations {
			break
		}

		// Phase 3: aggregate using refined partition (KEY Leiden difference vs. Louvain).
		prevGraph := currentGraph
		newGraph, superToRep := buildSupergraph(currentGraph, state.refinedPartition, &state.sgScratch)

		// Build reverse slice: refined community ID -> new supernode NodeID.
		// Refined partition IDs are in [0, currentGraph.NodeCount()), so a slice
		// indexed by comm ID replaces the map — zero-allocation after first pass.
		curN := currentGraph.NodeCount()
		if cap(state.commToNewSuperBuf) < curN {
			state.commToNewSuperBuf = make([]NodeID, curN)
		} else {
			state.commToNewSuperBuf = state.commToNewSuperBuf[:curN]
		}
		commToNewSuper := state.commToNewSuperBuf
		for i, rep := range superToRep {
			comm := state.refinedPartition[rep]
			commToNewSuper[comm] = NodeID(i)
		}

		// Update nodeMappingSlice in-place: each original node follows its current
		// supernode through refinedPartition then commToNewSuper to the new supernode.
		for i := range nodeMappingSlice {
			curSuper := nodeMappingSlice[i]
			nodeMappingSlice[i] = commToNewSuper[state.refinedPartition[curSuper]]
		}

		// Release replaced supergraph back to pool. Original g is caller-owned.
		// Scratch-owned supergraphs (superGraphA/B) must NOT be returned to graphPool —
		// they are reused by the next buildSupergraph call via the ping-pong mechanism.
		if prevGraph != g && prevGraph != state.sgScratch.superGraphA && prevGraph != state.sgScratch.superGraphB {
			releaseGraph(prevGraph)
		}
		currentGraph = newGraph
		buildCSRInto(currentGraph, &state.csrBuf)
		csr = state.csrBuf // header copy; backing arrays reused from state.csrBuf
		// If the supergraph has collapsed to a single node, we've fully converged.
		if currentGraph.NodeCount() <= 1 {
			break
		}
	}

	// --- Reconstruct final partition using best result found ---
	finalPartition := reconstructPartitionFromSlice(origNodes, bestNodeMappingSlice, bestSuperPartition)
	finalPartition = normalizePartitionWithBufs(finalPartition, &state.normUsedBuf, &state.normRemapBuf)
	q := ComputeModularityWeighted(g, finalPartition, resolution)

	return CommunityResult{
		Partition:  finalPartition,
		Modularity: q,
		Passes:     totalPasses,
		Moves:      totalMoves,
	}, nil
}

// refinePartitionInPlace splits disconnected communities via BFS, writing results
// directly into st.refinedPartition. Zero heap allocations after first pool warm-up.
//
// Phase 2 eliminated per-community map allocations (inComm/visited maps).
// Phase 3 optimizations:
//   - Counting sort replaces slices.SortFunc: O(N) grouping via community-size prefix sums
//     instead of O(N log N) comparison sort. commCountScratch is sparse-reset via commSeenComms.
//   - BFS queue stores int32 CSR dense indices instead of NodeIDs: csr.adjByIdx[curIdx]
//     replaces g.Neighbors(cur) (adjacency map lookup → direct slice access).
func refinePartitionInPlace(g *Graph, csr *csrGraph, partition map[NodeID]int, st *leidenState) {
	n := len(csr.nodeIDs)

	// Grow CSR-indexed scratch slices lazily (once per pool lifetime after first large graph).
	// Both slices are always grown together to keep them in sync.
	if len(st.inCommBits) < n || len(st.visitedBits) < n {
		st.inCommBits = make([]bool, n)
		st.visitedBits = make([]bool, n)
	}
	if len(st.commCountScratch) < n {
		st.commCountScratch = make([]int, n)
	}

	// --- Counting sort: group nodes by community in O(N) ---

	// Pass 1: collect (comm, node) pairs; count community sizes.
	// commCountScratch is pre-zeroed (reset at end of prior call via commSeenComms).
	// Invariant: partition IDs after phase1 are always in [0, n) because reset() assigns
	// cold-start IDs in [0, N) and phase1 only adopts existing IDs — never creates new ones.
	st.commBuildPairs = st.commBuildPairs[:0]
	st.commSeenComms = st.commSeenComms[:0]
	for node, comm := range partition {
		if comm < 0 || comm >= n {
			panic(fmt.Sprintf("refinePartitionInPlace: partition ID %d out of bounds [0, %d)", comm, n))
		}
		st.commBuildPairs = append(st.commBuildPairs, commNodePair{comm: comm, node: node})
		if st.commCountScratch[comm] == 0 {
			st.commSeenComms = append(st.commSeenComms, comm)
		}
		st.commCountScratch[comm]++
	}

	// Sort the community ID list (small: ~comms, not ~nodes) for deterministic processing order.
	slices.Sort(st.commSeenComms)

	// Compute exclusive prefix sums: commCountScratch[c] becomes the start offset
	// of community c in the scatter output buffer.
	np := len(st.commBuildPairs)
	if cap(st.commSortedPairs) < np {
		// Allocate with 25% headroom to amortize growth when node count increases.
		st.commSortedPairs = make([]commNodePair, np, np+np/4+1)
	} else {
		st.commSortedPairs = st.commSortedPairs[:np]
	}
	offset := 0
	for _, c := range st.commSeenComms {
		size := st.commCountScratch[c]
		st.commCountScratch[c] = offset
		offset += size
	}

	// Pass 2: scatter pairs into output buffer; commCountScratch[c] advances as cursor.
	for _, p := range st.commBuildPairs {
		pos := st.commCountScratch[p.comm]
		st.commSortedPairs[pos] = p
		st.commCountScratch[p.comm]++
	}

	// Reset commCountScratch for next call (sparse reset: only touched entries).
	for _, c := range st.commSeenComms {
		st.commCountScratch[c] = 0
	}

	// --- BFS refinement: one pass per connected component ---

	// Clear refined partition for this pass (reuse existing map allocation).
	clear(st.refinedPartition)

	nextID := 0

	// Process each community group. commSortedPairs is grouped by community in
	// ascending comm-ID order (scatter preserves commSeenComms sorted order).
	// The two orderings are coupled: commSeenComms is sorted, and the prefix-sum
	// scatter places community c's nodes at offsets [commOffset[c], commOffset[c]+size[c]).
	// Community boundaries are therefore contiguous, and the inner `end` scan is correct.
	start := 0
	for range st.commSeenComms {
		end := start
		for end < np && st.commSortedPairs[end].comm == st.commSortedPairs[start].comm {
			end++
		}
		// st.commSortedPairs[start:end] holds all nodes in this community.

		// Mark inComm bits for every node in this community.
		for _, p := range st.commSortedPairs[start:end] {
			st.inCommBits[csr.nodeToIdx(p.node)] = true
		}

		// BFS from each unvisited node — each BFS discovers one connected component.
		// Queue stores int32 CSR dense indices; adjIdxFlat provides (neighbor-idx, weight)
		// pairs with no map lookups — pure slice accesses throughout the BFS.
		for _, p := range st.commSortedPairs[start:end] {
			startIdx := csr.nodeToIdx(p.node)
			if st.visitedBits[startIdx] {
				continue
			}
			st.bfsQueue = st.bfsQueue[:0]
			st.bfsQueue = append(st.bfsQueue, startIdx)
			st.visitedBits[startIdx] = true
			head := 0
			for head < len(st.bfsQueue) {
				curIdx := st.bfsQueue[head]
				head++
				st.refinedPartition[csr.nodeIDs[curIdx]] = nextID
				for _, e := range csr.neighborsIdx(curIdx) {
					if e.ToIdx == curIdx {
						continue // skip self-loops
					}
					if !st.inCommBits[e.ToIdx] {
						continue // skip cross-community edges
					}
					if !st.visitedBits[e.ToIdx] {
						st.visitedBits[e.ToIdx] = true
						st.bfsQueue = append(st.bfsQueue, e.ToIdx)
					}
				}
			}
			nextID++
		}

		// Clear inComm and visited bits — only touches nodes in this community.
		for _, p := range st.commSortedPairs[start:end] {
			idx := csr.nodeToIdx(p.node)
			st.inCommBits[idx] = false
			st.visitedBits[idx] = false
		}

		start = end
	}
}
