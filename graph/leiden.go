package graph

import (
	"math"
	"slices"
	"time"
)

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
		moves := phase1(currentGraph, ls, resolution, currentGraph.TotalWeight())
		state.partition = ls.partition
		state.commStr = ls.commStr
		state.neighborDirty = ls.neighborDirty
		state.candidateBuf = ls.candidateBuf
		totalPasses++
		totalMoves += moves

		// Phase 2 (Leiden-specific): BFS refinement — split disconnected communities.
		state.refinedPartition = refinePartition(currentGraph, state.partition)

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

// refinePartition returns a new partition where each connected component
// within every community becomes its own community.
// Self-loops are skipped during BFS (they don't contribute to connectivity).
// Communities are processed in sorted order for deterministic output.
func refinePartition(g *Graph, partition map[NodeID]int) map[NodeID]int {
	// Group nodes by community.
	commNodes := make(map[int][]NodeID)
	for n, c := range partition {
		commNodes[c] = append(commNodes[c], n)
	}

	// Collect and sort community IDs for deterministic output.
	commIDs := make([]int, 0, len(commNodes))
	for c := range commNodes {
		commIDs = append(commIDs, c)
	}
	slices.Sort(commIDs)

	refined := make(map[NodeID]int, len(partition))
	nextID := 0

	for _, comm := range commIDs {
		nodes := commNodes[comm]
		// Build node-set for O(1) intra-community neighbor filtering.
		inComm := make(map[NodeID]struct{}, len(nodes))
		for _, n := range nodes {
			inComm[n] = struct{}{}
		}

		visited := make(map[NodeID]bool, len(nodes))
		for _, start := range nodes {
			if visited[start] {
				continue
			}
			// BFS: only traverse edges where the neighbor is in the same community.
			queue := []NodeID{start}
			visited[start] = true
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				refined[cur] = nextID
				for _, e := range g.Neighbors(cur) {
					if e.To == cur {
						continue // skip self-loops
					}
					if _, ok := inComm[e.To]; !ok {
						continue // skip cross-community edges
					}
					if !visited[e.To] {
						visited[e.To] = true
						queue = append(queue, e.To)
					}
				}
			}
			nextID++
		}
	}
	return refined
}
