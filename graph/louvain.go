package graph

import "slices"

// Detect runs the Louvain community detection algorithm on graph g.
// It returns ErrDirectedNotSupported for directed graphs.
// For empty graphs, it returns an empty CommunityResult with no error.
// The returned Partition is always 0-indexed contiguous.
func (d *louvainDetector) Detect(g *Graph) (CommunityResult, error) {
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

	// --- Resolve zero-value options ---
	resolution := d.opts.Resolution
	if resolution == 0.0 {
		resolution = 1.0
	}
	maxPasses := d.opts.MaxPasses // 0 = unlimited
	seed := d.opts.Seed           // 0 = random (handled inside newLouvainState)

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

	// superPartition holds the partition on the current (possibly compressed) supergraph
	// after the most recent phase1 pass.
	var superPartition map[NodeID]int

	// Best-partition tracking: retain the highest-Q partition found so far
	// to guard against degenerate convergence on later passes.
	bestQ := -1.0
	bestNodeMapping := make(map[NodeID]NodeID, len(origNodes))
	for _, n := range origNodes {
		bestNodeMapping[n] = n
	}
	var bestSuperPartition map[NodeID]int

	state := acquireLouvainState(currentGraph, seed)
	defer releaseLouvainState(state)

	for {
		state.reset(currentGraph, seed)
		moves := phase1(currentGraph, state, resolution, currentGraph.TotalWeight())
		totalPasses++
		totalMoves += moves

		superPartition = state.partition

		// Compute current partition quality to track best result.
		candidatePartition := reconstructPartition(origNodes, nodeMapping, superPartition)
		candidateQ := ComputeModularityWeighted(g, candidatePartition, resolution)
		if candidateQ > bestQ {
			bestQ = candidateQ
			for k, v := range nodeMapping {
				bestNodeMapping[k] = v
			}
			// Copy superPartition — state.partition is reused across loop iterations.
			bestSuperPartition = make(map[NodeID]int, len(superPartition))
			for k, v := range superPartition {
				bestSuperPartition[k] = v
			}
		}

		if moves == 0 {
			break
		}
		if maxPasses > 0 && totalPasses >= maxPasses {
			break
		}

		// Phase 2: compress communities into supernodes.
		newGraph, superToRep := buildSupergraph(currentGraph, superPartition)

		// Build reverse map: community ID (in currentGraph) -> new supernode NodeID.
		// superToRep maps newSuperNode -> representative node in currentGraph.
		// The community of that representative is: superPartition[rep].
		commToNewSuper := make(map[int]NodeID, len(superToRep))
		for newSuper, rep := range superToRep {
			comm := superPartition[rep]
			commToNewSuper[comm] = newSuper
		}

		// Update nodeMapping to point original nodes to the new supernodes.
		// Build into a new map to avoid mutating while reading.
		newMapping := make(map[NodeID]NodeID, len(nodeMapping))
		for orig, curSuper := range nodeMapping {
			comm := superPartition[curSuper]
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

// reconstructPartition maps original NodeIDs to their final community IDs
// by following nodeMapping and then superPartition.
func reconstructPartition(origNodes []NodeID, nodeMapping map[NodeID]NodeID, superPartition map[NodeID]int) map[NodeID]int {
	p := make(map[NodeID]int, len(origNodes))
	for _, orig := range origNodes {
		superNode := nodeMapping[orig]
		p[orig] = superPartition[superNode]
	}
	return p
}

// phase1 performs one full pass of local moves (Phase 1 of Louvain).
// Iterates over all nodes in shuffled order, moving each to the neighboring
// community with the highest modularity gain. Returns the number of moves made.
func phase1(g *Graph, state *louvainState, resolution, m float64) int {
	nodes := g.Nodes()
	// Sort by NodeID before shuffling so the RNG seed is the sole source of
	// traversal randomness (map iteration order is intentionally non-deterministic in Go).
	slices.Sort(nodes)
	state.rng.Shuffle(len(nodes), func(i, j int) {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	})

	moves := 0
	for _, n := range nodes {
		currentComm := state.partition[n]
		ki := g.Strength(n)

		// Remove n from its current community (temporarily).
		state.commStr[currentComm] -= ki

		// Single-pass neighbor accumulation:
		// Accumulate edge weights from n to each neighbor community into neighborBuf.
		// Also gather candidate communities (with dedup) into candidateBuf.
		// neighborDirty tracks keys written so we can reset without clearing the whole map.
		state.candidateBuf = state.candidateBuf[:0]
		for _, k := range state.neighborDirty {
			delete(state.neighborBuf, k)
		}
		state.neighborDirty = state.neighborDirty[:0]

		// Always include currentComm (even if no neighbors there after removal).
		if _, seen := state.neighborBuf[NodeID(currentComm)]; !seen {
			state.neighborBuf[NodeID(currentComm)] = 0.0
			state.neighborDirty = append(state.neighborDirty, NodeID(currentComm))
			state.candidateBuf = append(state.candidateBuf, currentComm)
		}
		for _, e := range g.Neighbors(n) {
			nc := state.partition[e.To]
			key := NodeID(nc)
			if _, seen := state.neighborBuf[key]; !seen {
				state.neighborBuf[key] = 0.0
				state.neighborDirty = append(state.neighborDirty, key)
				state.candidateBuf = append(state.candidateBuf, nc)
			}
			state.neighborBuf[key] += e.Weight
		}
		// neighborBuf[NodeID(comm)] now holds kiIn for community comm.

		candidates := state.candidateBuf
		// Insertion sort — candidate count is small (bounded by node degree).
		for i := 1; i < len(candidates); i++ {
			for j := i; j > 0 && candidates[j] < candidates[j-1]; j-- {
				candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
			}
		}

		// Inline ΔQ computation using precomputed kiIn from neighborBuf.
		// ΔQ(comm) = kiIn/m - resolution*(sigTot/(2m))*(ki/(2m))
		twoM := 2.0 * m
		kiOverTwoM := ki / twoM

		kiInCur := state.neighborBuf[NodeID(currentComm)]
		sigTotCur := state.commStr[currentComm]
		curDQ := kiInCur/m - resolution*(sigTotCur/twoM)*kiOverTwoM

		// Find best community (highest ΔQ gain over staying in currentComm).
		// Ties broken by lower community ID (candidates is sorted, so first seen wins).
		bestComm := currentComm
		bestGain := 0.0
		for _, comm := range candidates {
			if comm == currentComm {
				continue
			}
			kiIn := state.neighborBuf[NodeID(comm)]
			sigTot := state.commStr[comm]
			dq := kiIn/m - resolution*(sigTot/twoM)*kiOverTwoM
			if gain := dq - curDQ; gain > bestGain {
				bestGain = gain
				bestComm = comm
			}
		}

		// Assign node to best community and update cached strength.
		state.partition[n] = bestComm
		state.commStr[bestComm] += ki
		if bestComm != currentComm {
			moves++
		}
	}
	return moves
}

// deltaQ computes the modularity gain of placing node n into community comm.
// Formula: kiIn/m - resolution * (sigTot/(2m)) * (ki/(2m))
// where m = TotalWeight(), sigTot = commStr[comm] (excluding n's contribution), kiIn = edge weight from n to comm.
func deltaQ(g *Graph, n NodeID, comm int, partition map[NodeID]int,
	commStr map[int]float64, resolution, m float64) float64 {
	kiIn := g.WeightToComm(n, comm, partition)
	sigTot := commStr[comm]
	ki := g.Strength(n)
	twoM := 2.0 * m
	return kiIn/m - resolution*(sigTot/twoM)*(ki/twoM)
}

// buildSupergraph compresses the current graph by merging each community into a supernode.
// Returns:
//   - newGraph: undirected weighted graph with one node per community
//   - superToRep: maps each new supernode NodeID -> a representative NodeID from g
func buildSupergraph(g *Graph, partition map[NodeID]int) (*Graph, map[NodeID]NodeID) {
	// Collect distinct community IDs and sort for deterministic supernode assignment.
	commSet := make(map[int]struct{})
	for _, comm := range partition {
		commSet[comm] = struct{}{}
	}
	commList := make([]int, 0, len(commSet))
	for comm := range commSet {
		commList = append(commList, comm)
	}
	slices.Sort(commList)

	// Assign contiguous supernode IDs in sorted community order.
	commToSuper := make(map[int]NodeID, len(commList))
	for i, comm := range commList {
		commToSuper[comm] = NodeID(i)
	}

	// Build superToRep: each supernode -> a representative original node.
	superToRep := make(map[NodeID]NodeID, len(commToSuper))
	for _, n := range g.Nodes() {
		comm := partition[n]
		super := commToSuper[comm]
		if _, exists := superToRep[super]; !exists {
			superToRep[super] = n
		}
	}

	// Accumulate inter-community edge weights using canonical (min,max) key to avoid double-counting.
	type edgeKey struct{ a, b NodeID }
	interEdges := make(map[edgeKey]float64)
	// Intra-community edges become self-loops on the supernode.
	selfLoops := make(map[NodeID]float64)

	for _, n := range g.Nodes() {
		superN := commToSuper[partition[n]]
		for _, e := range g.Neighbors(n) {
			superNeighbor := commToSuper[partition[e.To]]
			if superN == superNeighbor {
				// Each undirected intra-community edge appears in adjacency from both endpoints.
				selfLoops[superN] += e.Weight
			} else {
				// Canonicalize key so (a,b) and (b,a) are the same.
				a, b := superN, superNeighbor
				if a > b {
					a, b = b, a
				}
				interEdges[edgeKey{a, b}] += e.Weight
			}
		}
	}

	newGraph := NewGraph(false)

	// Self-loops: each undirected intra-edge counted twice in adjacency → divide by 2.
	for super, w := range selfLoops {
		newGraph.AddEdge(super, super, w/2.0)
	}

	// Inter-community edges: each undirected edge between communities was counted from both
	// endpoints (a→b and b→a), so divide accumulated weight by 2.
	for key, w := range interEdges {
		newGraph.AddEdge(key.a, key.b, w/2.0)
	}

	// Ensure all supernodes exist even if isolated (no edges).
	for _, super := range commToSuper {
		if _, exists := newGraph.nodes[super]; !exists {
			newGraph.AddNode(super, 1.0)
		}
	}

	return newGraph, superToRep
}

// normalizePartition remaps community IDs to 0-indexed contiguous integers.
// Assignment order is determined by ascending NodeID for reproducibility.
func normalizePartition(partition map[NodeID]int) map[NodeID]int {
	// Sort nodes by NodeID for deterministic remap order.
	nodes := make([]NodeID, 0, len(partition))
	for n := range partition {
		nodes = append(nodes, n)
	}
	// Use slices.Sort (O(n log n)) — insertion sort is too slow for large graphs.
	slices.Sort(nodes)

	remap := make(map[int]int)
	next := 0
	normalized := make(map[NodeID]int, len(partition))
	for _, n := range nodes {
		c := partition[n]
		if _, exists := remap[c]; !exists {
			remap[c] = next
			next++
		}
		normalized[n] = remap[c]
	}
	return normalized
}
