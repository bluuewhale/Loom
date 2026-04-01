package graph

import (
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
// directly into st.refinedPartition. It avoids all per-community heap allocations
// by using CSR-indexed boolean scratch arrays and a single sorted-pairs pass.
//
// Replaces the old refinePartition which allocated commNodes, inComm, and visited
// maps per community — the dominant allocation source in the Leiden 10K benchmark.
// Self-loops are skipped during BFS. Communities are processed in sorted comm-ID
// order; nodes within each community are processed in sorted NodeID order for
// deterministic BFS starts.
func refinePartitionInPlace(g *Graph, csr *csrGraph, partition map[NodeID]int, st *leidenState) {
	n := len(csr.nodeIDs)

	// Grow CSR-indexed scratch slices lazily (once per pool lifetime after first large graph).
	// Both slices are always grown together to keep them in sync.
	if len(st.inCommBits) < n || len(st.visitedBits) < n {
		st.inCommBits = make([]bool, n)
		st.visitedBits = make([]bool, n)
	}

	// Build (comm, node) pairs — one per node, reuse backing array.
	// Reset to length 0 before appending so no stale entries from prior larger graphs
	// can contaminate the sort when cap >= n.
	st.commBuildPairs = st.commBuildPairs[:0]
	for node, comm := range partition {
		st.commBuildPairs = append(st.commBuildPairs, commNodePair{comm: comm, node: node})
	}

	// Sort by (community ID, node ID) — communities in sorted order, nodes within
	// each community in deterministic order for reproducible BFS start selection.
	// Use explicit three-way compare for NodeID to avoid truncation if the type widens.
	slices.SortFunc(st.commBuildPairs, func(a, b commNodePair) int {
		if a.comm != b.comm {
			return a.comm - b.comm
		}
		if a.node < b.node {
			return -1
		}
		if a.node > b.node {
			return 1
		}
		return 0
	})

	// Clear refined partition for this pass (reuse existing map allocation).
	clear(st.refinedPartition)

	nextID := 0
	var queue []NodeID // backing reused across communities

	// Process each community group in sorted order.
	np := len(st.commBuildPairs) // use actual pair count, not stale n
	for start := 0; start < np; {
		comm := st.commBuildPairs[start].comm
		end := start
		for end < np && st.commBuildPairs[end].comm == comm {
			end++
		}
		// st.commBuildPairs[start:end] holds all nodes in this community.

		// Mark inComm bits for every node in this community.
		for _, p := range st.commBuildPairs[start:end] {
			st.inCommBits[csr.idToIdx[p.node]] = true
		}

		// BFS from each unvisited node — each BFS discovers one connected component.
		for _, p := range st.commBuildPairs[start:end] {
			startIdx := csr.idToIdx[p.node]
			if st.visitedBits[startIdx] {
				continue
			}
			queue = queue[:0]
			queue = append(queue, p.node)
			st.visitedBits[startIdx] = true
			head := 0
			for head < len(queue) {
				cur := queue[head]
				head++
				st.refinedPartition[cur] = nextID
				for _, e := range g.Neighbors(cur) {
					if e.To == cur {
						continue // skip self-loops
					}
					toIdx := csr.idToIdx[e.To]
					if !st.inCommBits[toIdx] {
						continue // skip cross-community edges
					}
					if !st.visitedBits[toIdx] {
						st.visitedBits[toIdx] = true
						queue = append(queue, e.To)
					}
				}
			}
			nextID++
		}

		// Clear inComm and visited bits — only touches nodes in this community.
		for _, p := range st.commBuildPairs[start:end] {
			idx := csr.idToIdx[p.node]
			st.inCommBits[idx] = false
			st.visitedBits[idx] = false
		}

		start = end
	}
}
