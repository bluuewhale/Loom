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

	// Acquire state early so csrBuf and sgScratch are available for the initial CSR build.
	currentGraph := g
	state := acquireLouvainState(currentGraph, seed)
	defer releaseLouvainState(state)

	// nodeMappingSlice tracks origNodes[i] → current supernode NodeID.
	// Slice copy/range eliminates the map range and map lookup overhead of the old
	// map[NodeID]NodeID approach for the small ego-nets dominating the hot path.
	origNodes := g.Nodes()
	sz := len(origNodes)
	if cap(state.nodeMappingSliceA) < sz {
		state.nodeMappingSliceA = make([]NodeID, sz, sz+sz/4+1)
	} else {
		state.nodeMappingSliceA = state.nodeMappingSliceA[:sz]
	}
	copy(state.nodeMappingSliceA, origNodes) // identity mapping
	nodeMappingSlice := state.nodeMappingSliceA

	buildCSRInto(currentGraph, &state.csrBuf)
	csr := state.csrBuf // cheap header copy; backing arrays shared with state.csrBuf
	totalPasses := 0
	totalMoves := 0

	// superPartition holds the partition on the current (possibly compressed) supergraph
	// after the most recent phase1 pass.
	var superPartition map[NodeID]int

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
		moves := phase1(currentGraph, &csr, state, resolution, currentGraph.TotalWeight())
		totalPasses++
		totalMoves += moves

		superPartition = state.partition

		// Compute current partition quality to track best result.
		// Reuse scratchPartition to avoid allocating a new N-entry map every pass.
		reconstructPartitionIntoSlice(origNodes, nodeMappingSlice, superPartition, state.scratchPartition)
		candidateQ := ComputeModularityWeighted(g, state.scratchPartition, resolution)
		if candidateQ > bestQ {
			bestQ = candidateQ
			copy(bestNodeMappingSlice, nodeMappingSlice) // O(N) slice copy; no map overhead
			// Copy superPartition into pooled map — state.partition reused each loop.
			clear(bestSuperPartition)
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
		prevGraph := currentGraph
		newGraph, superToRep := buildSupergraph(currentGraph, superPartition, &state.sgScratch)

		// Build reverse slice: community ID (in currentGraph) -> new supernode NodeID.
		// Partition IDs after phase1 are in [0, currentGraph.NodeCount()), so a slice
		// indexed by comm ID replaces the map — zero-allocation after first pass.
		curN := currentGraph.NodeCount()
		if cap(state.commToNewSuperBuf) < curN {
			state.commToNewSuperBuf = make([]NodeID, curN)
		} else {
			state.commToNewSuperBuf = state.commToNewSuperBuf[:curN]
		}
		commToNewSuper := state.commToNewSuperBuf
		for i, rep := range superToRep {
			comm := superPartition[rep]
			commToNewSuper[comm] = NodeID(i)
		}

		// Update nodeMappingSlice in-place: for each original node, follow its current
		// supernode through superPartition then commToNewSuper to reach the new supernode.
		for i := range nodeMappingSlice {
			curSuper := nodeMappingSlice[i]
			nodeMappingSlice[i] = commToNewSuper[superPartition[curSuper]]
		}

		// Release replaced supergraph back to pool. Original g is caller-owned.
		if prevGraph != g {
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

// reconstructPartitionInto is like reconstructPartition but writes into dst instead of
// allocating a new map. dst is cleared before use. Use this with a pooled scratch map
// to eliminate per-pass map allocations during the Leiden supergraph iteration loop.
func reconstructPartitionInto(origNodes []NodeID, nodeMapping map[NodeID]NodeID, superPartition map[NodeID]int, dst map[NodeID]int) {
	clear(dst)
	for _, orig := range origNodes {
		superNode := nodeMapping[orig]
		dst[orig] = superPartition[superNode]
	}
}

// reconstructPartitionIntoSlice is like reconstructPartitionInto but uses a slice-indexed
// nodeMapping (position i → supernode for origNodes[i]) to eliminate per-node map lookups.
func reconstructPartitionIntoSlice(origNodes []NodeID, nodeMappingSlice []NodeID, superPartition map[NodeID]int, dst map[NodeID]int) {
	clear(dst)
	for i, orig := range origNodes {
		dst[orig] = superPartition[nodeMappingSlice[i]]
	}
}

// reconstructPartitionFromSlice allocates a fresh result map using a slice-indexed nodeMapping.
func reconstructPartitionFromSlice(origNodes []NodeID, nodeMappingSlice []NodeID, superPartition map[NodeID]int) map[NodeID]int {
	p := make(map[NodeID]int, len(origNodes))
	for i, orig := range origNodes {
		p[orig] = superPartition[nodeMappingSlice[i]]
	}
	return p
}

// phase1 performs one full pass of local moves (Phase 1 of Louvain).
// Iterates over all nodes in shuffled order, moving each to the neighboring
// community with the highest modularity gain. Returns the number of moves made.
// csr provides O(1) indexed neighbor lookups; phase1 shuffles dense indices
// to avoid the idToIdx map lookup in the hot loop.
//
// neighborBuf and commStr are flat []float64 slices indexed by community ID
// (always in [0,n)); neighborSeen is a parallel []bool for O(1) dedup.
// partitionByIdx mirrors state.partition but is indexed by CSR dense index,
// eliminating hash lookups from the inner neighbor loop entirely.
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

	// Build partitionByIdx: CSR dense-index -> community ID.
	// One sequential pass over state.partition (O(n) map reads, done once per phase1 call).
	// All subsequent community lookups in the inner loop use slice accesses only.
	pbIdx := state.partitionByIdx
	if len(pbIdx) < n {
		pbIdx = make([]int, n)
		state.partitionByIdx = pbIdx
	}
	for i, nid := range csr.nodeIDs {
		pbIdx[i] = state.partition[nid]
	}

	moves := 0
	for _, idx := range indices {
		node := csr.nodeIDs[idx]
		currentComm := pbIdx[idx]
		ki := csr.strength(idx)

		// Remove node from its current community (temporarily).
		state.commStr[currentComm] -= ki

		// Clean up neighborBuf/neighborSeen from the previous node (dirty-list reset).
		state.candidateBuf = state.candidateBuf[:0]
		for _, c := range state.neighborDirty {
			state.neighborBuf[c] = 0.0
			state.neighborSeen[c] = false
		}
		state.neighborDirty = state.neighborDirty[:0]

		// Always include currentComm (even if no neighbors remain there after removal).
		if !state.neighborSeen[currentComm] {
			state.neighborSeen[currentComm] = true
			state.neighborDirty = append(state.neighborDirty, currentComm)
			state.candidateBuf = append(state.candidateBuf, currentComm)
		}
		for _, e := range csr.neighborsIdx(idx) {
			nc := pbIdx[e.ToIdx] // slice access — no map lookup
			if !state.neighborSeen[nc] {
				state.neighborSeen[nc] = true
				state.neighborDirty = append(state.neighborDirty, nc)
				state.candidateBuf = append(state.candidateBuf, nc)
			}
			state.neighborBuf[nc] += e.Weight
		}
		// neighborBuf[comm] now holds kiIn (edge weight from node to community comm).

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

		kiInCur := state.neighborBuf[currentComm]
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
			kiIn := state.neighborBuf[comm]
			sigTot := state.commStr[comm]
			dq := kiIn/m - resolution*(sigTot/twoM)*kiOverTwoM
			if gain := dq - curDQ; gain > bestGain {
				bestGain = gain
				bestComm = comm
			}
		}

		// Assign node to best community and update cached strength.
		state.partition[node] = bestComm
		pbIdx[idx] = bestComm // keep idx-cache in sync
		state.commStr[bestComm] += ki
		if bestComm != currentComm {
			moves++
		}
	}
	return moves
}

// edgeKey is the canonical (a ≤ b) pair used to deduplicate inter-community edges
// in buildSupergraph. Lifted to package scope so supergraphScratch can reference it.
type edgeKey struct{ a, b NodeID }

// supergraphScratch holds reusable buffers for buildSupergraph.
// Embedding one of these in louvainState/leidenState eliminates ~12 allocs per call:
// all temporary slices and the interEdges map are cleared and reused across passes
// instead of being allocated fresh each time.
type supergraphScratch struct {
	occupied    []bool
	commToSuper []NodeID
	commList    []int
	superToRep  []NodeID
	assigned    []bool
	selfLoops   []float64
	interDegree []int
	interEdges  map[edgeKey]float64
	interKeys   []edgeKey
	// edgeBacking and sortedSuperIDs are reused across passes within one Louvain/Leiden run.
	// Safety: both are overwritten AFTER all reads from the previous supergraph's adjacency/sortedNodes.
	edgeBacking   []Edge   // backing array for newGraph adjacency slices
	sortedSuperIDs []NodeID // pre-cached sortedNodes for newGraph (IDs 0..N-1)
}

// buildSupergraph compresses the current graph by merging each community into a supernode.
// Returns:
//   - newGraph: undirected weighted graph with one node per community
//   - superToRep: superToRep[i] = representative NodeID from g for supernode NodeID(i)
//
// sc holds reusable scratch buffers; all slices and the interEdges map are cleared and
// reused across passes to eliminate per-call allocations.
func buildSupergraph(g *Graph, partition map[NodeID]int, sc *supergraphScratch) (*Graph, []NodeID) {
	n := len(partition) // partition IDs are in [0, n) by invariant

	// Grow and zero occupied/commToSuper lazily (bool slices are zero on make;
	// reused slices retain prior values so we zero exactly [0,n) before use).
	if cap(sc.occupied) < n {
		sc.occupied = make([]bool, n)
		sc.commToSuper = make([]NodeID, n)
	} else {
		sc.occupied = sc.occupied[:n]
		clear(sc.occupied)
		sc.commToSuper = sc.commToSuper[:n]
	}
	occupied := sc.occupied
	commToSuper := sc.commToSuper

	for _, comm := range partition {
		occupied[comm] = true
	}

	// Build sorted commList and commToSuper (slice, index = comm ID).
	// Scanning occupied[] in ascending order gives a sorted commList automatically.
	communityCount := 0
	sc.commList = sc.commList[:0]
	for i, occ := range occupied {
		if occ {
			commToSuper[i] = NodeID(communityCount)
			sc.commList = append(sc.commList, i)
			communityCount++
		}
	}

	// Build superToRep: superToRep[superID] = a representative original node.
	if cap(sc.superToRep) < communityCount {
		sc.superToRep = make([]NodeID, communityCount)
		sc.assigned = make([]bool, communityCount)
	} else {
		sc.superToRep = sc.superToRep[:communityCount]
		sc.assigned = sc.assigned[:communityCount]
		clear(sc.assigned)
	}
	superToRep := sc.superToRep
	assigned := sc.assigned
	for _, nd := range g.Nodes() {
		super := int(commToSuper[partition[nd]])
		if !assigned[super] {
			superToRep[super] = nd
			assigned[super] = true
		}
	}

	// Accumulate inter-community edge weights (map for sparse (a,b) pairs).
	// Reuse interEdges map: clear retains capacity, avoiding re-alloc of bucket array.
	if sc.interEdges == nil {
		sc.interEdges = make(map[edgeKey]float64, g.EdgeCount())
	} else {
		clear(sc.interEdges)
	}
	interEdges := sc.interEdges

	// Grow selfLoops lazily; zero only the [0, communityCount) prefix.
	if cap(sc.selfLoops) < communityCount {
		sc.selfLoops = make([]float64, communityCount)
	} else {
		sc.selfLoops = sc.selfLoops[:communityCount]
		clear(sc.selfLoops)
	}
	selfLoops := sc.selfLoops

	for _, nd := range g.Nodes() {
		superN := commToSuper[partition[nd]]
		for _, e := range g.Neighbors(nd) {
			superNeighbor := commToSuper[partition[e.To]]
			if superN == superNeighbor {
				selfLoops[superN] += e.Weight
			} else {
				a, b := superN, superNeighbor
				if a > b {
					a, b = b, a
				}
				interEdges[edgeKey{a, b}] += e.Weight
			}
		}
	}

	// Pre-compute per-supernode inter-community degree.
	if cap(sc.interDegree) < communityCount {
		sc.interDegree = make([]int, communityCount)
	} else {
		sc.interDegree = sc.interDegree[:communityCount]
		clear(sc.interDegree)
	}
	interDegree := sc.interDegree
	for key := range interEdges {
		interDegree[int(key.a)]++
		interDegree[int(key.b)]++
	}

	// Compute total edge slots for the single backing array.
	totalEdgeCap := 0
	for i := range communityCount {
		totalEdgeCap += interDegree[i]
		if selfLoops[i] > 0 {
			totalEdgeCap++
		}
	}
	// Reuse edgeBacking across passes: all reads of the previous supergraph's adjacency
	// are complete before we write here, so overwriting the backing is safe.
	if cap(sc.edgeBacking) < totalEdgeCap {
		sc.edgeBacking = make([]Edge, totalEdgeCap, totalEdgeCap+totalEdgeCap/4+1)
	} else {
		sc.edgeBacking = sc.edgeBacking[:totalEdgeCap]
	}
	edgeBacking := sc.edgeBacking

	newGraph := newGraphSized(false, communityCount)
	off := 0
	for i := range communityCount {
		super := NodeID(i)
		capNeeded := interDegree[i]
		if selfLoops[i] > 0 {
			capNeeded++
		}
		newGraph.nodes[super] = 1.0
		if capNeeded > 0 {
			newGraph.adjacency[super] = edgeBacking[off : off : off+capNeeded]
			off += capNeeded
		}
	}

	// Self-loops: intra-edge counted twice (both directions) → divide by 2.
	// Iterate in ascending supernode order for deterministic layout.
	for i := range communityCount {
		if selfLoops[i] > 0 {
			newGraph.AddEdge(NodeID(i), NodeID(i), selfLoops[i]/2.0)
		}
	}

	// Inter-community edges: counted from both endpoints → divide by 2.
	sc.interKeys = sc.interKeys[:0]
	for key := range interEdges {
		sc.interKeys = append(sc.interKeys, key)
	}
	interKeys := sc.interKeys
	slices.SortFunc(interKeys, func(x, y edgeKey) int {
		if x.a != y.a {
			return int(x.a) - int(y.a)
		}
		return int(x.b) - int(y.b)
	})
	for _, key := range interKeys {
		newGraph.AddEdge(key.a, key.b, interEdges[key]/2.0)
	}

	// Pre-cache sortedNodes: supergraph NodeIDs are always 0..communityCount-1.
	// Reuse sc.sortedSuperIDs across passes — safe because all reads of the old
	// supergraph's sortedNodes finish before this overwrite.
	if cap(sc.sortedSuperIDs) < communityCount {
		sc.sortedSuperIDs = make([]NodeID, communityCount, communityCount+communityCount/4+1)
	} else {
		sc.sortedSuperIDs = sc.sortedSuperIDs[:communityCount]
	}
	for i := range communityCount {
		sc.sortedSuperIDs[i] = NodeID(i)
	}
	newGraph.sortedNodes = sc.sortedSuperIDs

	return newGraph, superToRep
}

// normalizePartitionWithBufs is like normalizePartition but reuses caller-provided
// scratch slices, eliminating the two allocs per call after pool warm-up.
func normalizePartitionWithBufs(partition map[NodeID]int, usedBuf *[]bool, remapBuf *[]int) map[NodeID]int {
	if len(partition) == 0 {
		return partition
	}
	maxComm := 0
	for _, c := range partition {
		if c > maxComm {
			maxComm = c
		}
	}
	need := maxComm + 1
	if cap(*usedBuf) >= need {
		*usedBuf = (*usedBuf)[:need]
		clear(*usedBuf)
	} else {
		*usedBuf = make([]bool, need)
	}
	if cap(*remapBuf) >= need {
		*remapBuf = (*remapBuf)[:need]
	} else {
		*remapBuf = make([]int, need)
	}
	used := *usedBuf
	remap := *remapBuf
	for _, c := range partition {
		used[c] = true
	}
	next := 0
	for i, u := range used {
		if u {
			remap[i] = next
			next++
		}
	}
	for n := range partition {
		partition[n] = remap[partition[n]]
	}
	return partition
}

// normalizePartition remaps community IDs to 0-indexed contiguous integers.
// Assignment order is determined by ascending NodeID for reproducibility.
func normalizePartition(partition map[NodeID]int) map[NodeID]int {
	if len(partition) == 0 {
		return partition
	}
	// Find max community ID to size the slice-based remap.
	maxComm := 0
	for _, c := range partition {
		if c > maxComm {
			maxComm = c
		}
	}

	// Mark which community IDs are occupied in ascending order.
	used := make([]bool, maxComm+1)
	for _, c := range partition {
		used[c] = true
	}

	// Assign new contiguous IDs in ascending old-comm-ID order (deterministic).
	// This replaces the original sort-by-NodeID approach but produces an equivalent
	// 0-indexed contiguous partition (community structure is identical).
	remap := make([]int, maxComm+1)
	next := 0
	for i, u := range used {
		if u {
			remap[i] = next
			next++
		}
	}

	// Apply remap in-place — avoids allocating a separate normalized map.
	for n := range partition {
		partition[n] = remap[partition[n]]
	}
	return partition
}
