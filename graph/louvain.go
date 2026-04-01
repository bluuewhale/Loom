package graph

import (
	"math"
	"slices"
)

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
	seed := d.opts.Seed           // 0 = random (handled inside acquireLouvainState)

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

	// superPartition holds the partition on the current (possibly compressed) supergraph
	// after the most recent phase1 pass.
	var superPartition map[NodeID]int

	// Best-partition tracking: retain the highest-Q partition found so far
	// to guard against degenerate convergence on later passes.
	bestQ := math.Inf(-1)
	bestNodeMapping := make(map[NodeID]NodeID, len(origNodes))
	for _, n := range origNodes {
		bestNodeMapping[n] = n
	}
	var bestSuperPartition map[NodeID]int

	state := acquireLouvainState(currentGraph, seed)
	defer releaseLouvainState(state)

	firstPass := true
	for {
		if firstPass {
			state.reset(currentGraph, seed, d.opts.InitialPartition)
			firstPass = false
		} else {
			state.reset(currentGraph, seed, nil)
		}
		moves := phase1(currentGraph, &csr, state, resolution, currentGraph.TotalWeight())
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
// csr provides O(1) indexed neighbor lookups; phase1 shuffles dense indices
// to avoid the idToIdx map lookup in the hot loop.
func phase1(g *Graph, csr *csrGraph, state *louvainState, resolution, m float64) int {
	n := len(csr.nodeIDs)
	// Shuffle dense indices [0..n-1] so we can use idx directly for csr lookups,
	// deriving NodeID via csr.nodeIDs[idx]. Avoids idToIdx map lookup per node.
	indices := state.idxBuf[:0]
	if cap(indices) >= n {
		indices = indices[:n]
	} else {
		indices = make([]int32, n)
		state.idxBuf = indices
	}
	for i := range indices {
		indices[i] = int32(i)
	}
	state.rng.Shuffle(n, func(i, j int) {
		indices[i], indices[j] = indices[j], indices[i]
	})

	moves := 0
	for _, idx := range indices {
		n := csr.nodeIDs[idx]
		currentComm := state.partition[n]
		ki := csr.strength(idx)

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
		for _, e := range csr.neighbors(idx) {
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

	// Accumulate inter-community edge weights using canonical (min,max) super-node key.
	// Both directions of each undirected edge are visited; divide accumulated weight by 2
	// when writing so each edge appears once at the correct weight.
	// Maps are pre-sized to reduce rehash overhead.
	type edgeKey struct{ a, b NodeID }
	interEdges := make(map[edgeKey]float64, g.EdgeCount())
	// Intra-community edges become self-loops on the supernode.
	selfLoops := make(map[NodeID]float64, len(commList))

	for _, n := range g.Nodes() {
		superN := commToSuper[partition[n]]
		for _, e := range g.Neighbors(n) {
			superNeighbor := commToSuper[partition[e.To]]
			if superN == superNeighbor {
				// Each undirected intra-community edge appears in adjacency from both endpoints.
				selfLoops[superN] += e.Weight
			} else {
				// Canonicalize key so (a,b) and (b,a) map to the same entry.
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
	// Write in sorted supernode order for deterministic adjacency layout.
	selfLoopNodes := make([]NodeID, 0, len(selfLoops))
	for super := range selfLoops {
		selfLoopNodes = append(selfLoopNodes, super)
	}
	slices.Sort(selfLoopNodes)
	for _, super := range selfLoopNodes {
		newGraph.AddEdge(super, super, selfLoops[super]/2.0)
	}

	// Inter-community edges: each undirected edge between communities was counted from both
	// endpoints (a→b and b→a), so divide accumulated weight by 2.
	// Write in sorted key order for deterministic adjacency layout.
	interKeys := make([]edgeKey, 0, len(interEdges))
	for key := range interEdges {
		interKeys = append(interKeys, key)
	}
	slices.SortFunc(interKeys, func(x, y edgeKey) int {
		if x.a != y.a {
			return int(x.a) - int(y.a)
		}
		return int(x.b) - int(y.b)
	})
	for _, key := range interKeys {
		newGraph.AddEdge(key.a, key.b, interEdges[key]/2.0)
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
