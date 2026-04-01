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

	// nodeMapping maps each original NodeID to its corresponding supernode NodeID
	// in the current supergraph. Initially the identity mapping.
	origNodes := g.Nodes()
	nodeMapping := make(map[NodeID]NodeID, len(origNodes))
	for _, n := range origNodes {
		nodeMapping[n] = n
	}

	currentGraph := g
	csr := buildCSR(currentGraph)
	totalPasses := 0
	totalMoves := 0

	// Best-partition tracking: retain the highest-Q partition found so far
	// to guard against degenerate convergence on later passes.
	bestQ := math.Inf(-1)
	bestNodeMapping := make(map[NodeID]NodeID, len(origNodes))
	for _, n := range origNodes {
		bestNodeMapping[n] = n
	}
	var bestSuperPartition map[NodeID]int

	state := acquireLeidenState(currentGraph, seed)
	defer releaseLeidenState(state)

	firstPass := true
	for {
		if firstPass {
			state.reset(currentGraph, seed, d.opts.InitialPartition)
			firstPass = false
		} else {
			state.reset(currentGraph, seed, nil)
		}

		// Phase 1: local move — reuse Louvain phase1 via louvainState wrapper.
		ls := &louvainState{
			partition:     state.partition,
			commStr:       state.commStr,
			neighborBuf:   state.neighborBuf,
			neighborDirty: state.neighborDirty,
			candidateBuf:  state.candidateBuf,
			rng:           state.rng,
		}
		moves := phase1(currentGraph, &csr, ls, resolution, currentGraph.TotalWeight())
		state.partition = ls.partition
		state.commStr = ls.commStr
		state.neighborDirty = ls.neighborDirty
		state.candidateBuf = ls.candidateBuf
		totalPasses++
		totalMoves += moves

		// Phase 2 (Leiden-specific): BFS refinement — split disconnected communities.
		refinePartitionInPlace(currentGraph, &csr, state.partition, state)

		// Best-Q tracking using refinedPartition (reflects actual aggregation structure).
		candidatePartition := reconstructPartition(origNodes, nodeMapping, state.refinedPartition)
		candidateQ := ComputeModularityWeighted(g, candidatePartition, resolution)
		if candidateQ > bestQ {
			bestQ = candidateQ
			for k, v := range nodeMapping {
				bestNodeMapping[k] = v
			}
			// Copy refinedPartition — state maps are cleared on reset each iteration.
			bestSuperPartition = make(map[NodeID]int, len(state.refinedPartition))
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
		newGraph, superToRep := buildSupergraph(currentGraph, state.refinedPartition)

		// Build reverse map: refined community ID -> new supernode NodeID.
		// superToRep maps newSuperNode -> representative node in currentGraph.
		// The community of that representative is: state.refinedPartition[rep].
		commToNewSuper := make(map[int]NodeID, len(superToRep))
		for newSuper, rep := range superToRep {
			comm := state.refinedPartition[rep]
			commToNewSuper[comm] = newSuper
		}

		// Update nodeMapping to point original nodes to the new supernodes.
		// Use refinedPartition (not partition) for consistency with aggregation.
		newMapping := make(map[NodeID]NodeID, len(nodeMapping))
		for orig, curSuper := range nodeMapping {
			comm := state.refinedPartition[curSuper]
			newMapping[orig] = commToNewSuper[comm]
		}
		nodeMapping = newMapping

		currentGraph = newGraph
		csr = buildCSR(currentGraph)
		// If the supergraph has collapsed to a single node, we've fully converged.
		if currentGraph.NodeCount() <= 1 {
			break
		}
	}

	// --- Reconstruct final partition using best result found ---
	finalPartition := reconstructPartition(origNodes, bestNodeMapping, bestSuperPartition)
	finalPartition = normalizePartition(finalPartition)
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
			st.inCommBits[csr.idToIdx[p.node]] = true
		}

		// BFS from each unvisited node — each BFS discovers one connected component.
		// Queue stores int32 CSR dense indices: csr.adjByIdx[idx] is a direct slice
		// access, replacing the g.Neighbors() adjacency map lookup.
		for _, p := range st.commSortedPairs[start:end] {
			startIdx := csr.idToIdx[p.node]
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
				cur := csr.nodeIDs[curIdx]
				st.refinedPartition[cur] = nextID
				for _, e := range csr.adjByIdx[curIdx] { // direct slice: no map lookup
					if e.To == cur {
						continue // skip self-loops
					}
					toIdx := csr.idToIdx[e.To]
					if !st.inCommBits[toIdx] {
						continue // skip cross-community edges
					}
					if !st.visitedBits[toIdx] {
						st.visitedBits[toIdx] = true
						st.bfsQueue = append(st.bfsQueue, toIdx)
					}
				}
			}
			nextID++
		}

		// Clear inComm and visited bits — only touches nodes in this community.
		for _, p := range st.commSortedPairs[start:end] {
			idx := csr.idToIdx[p.node]
			st.inCommBits[idx] = false
			st.visitedBits[idx] = false
		}

		start = end
	}
}
