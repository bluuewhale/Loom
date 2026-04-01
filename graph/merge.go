package graph

import (
	"errors"
	"math"
)

// ErrInvalidMergeOptions is returned when MergeOptions contains invalid values.
var ErrInvalidMergeOptions = errors.New("invalid merge options")

// ErrPartitionGraphMismatch is returned when the partition references node IDs
// not present in the graph.
var ErrPartitionGraphMismatch = errors.New("partition contains node IDs not present in graph")

// MergeStrategy controls how a small community selects its merge target.
type MergeStrategy int

const (
	// MergeByConnectivity merges into the neighbouring community with the
	// highest total edge weight. O(edges in small community) per decision.
	MergeByConnectivity MergeStrategy = iota

	// MergeByModularity merges into the neighbour that yields the greatest
	// modularity gain (or least loss).
	MergeByModularity
)

// MergeOptions configures small-community merging.
// Zero value is valid: MinSize=0 and MinFraction=0.0 → no merging performed.
type MergeOptions struct {
	// MinSize: communities with fewer than MinSize nodes are merge candidates.
	MinSize int

	// MinFraction: communities smaller than MinFraction * totalNodes are
	// merge candidates (OR condition with MinSize).
	MinFraction float64

	// Strategy selects the merge-target rule. Default: MergeByConnectivity.
	Strategy MergeStrategy

	// Resolution scales the modularity penalty term (MergeByModularity only).
	// Zero value treated as 1.0. To express a resolution near zero, use a
	// small positive value (e.g. 1e-9); exactly 0.0 cannot be expressed.
	Resolution float64
}

// validateMergeOptions returns ErrInvalidMergeOptions for out-of-range values.
func validateMergeOptions(opts MergeOptions) error {
	if opts.MinSize < 0 || opts.MinFraction < 0 || opts.MinFraction > 1 {
		return ErrInvalidMergeOptions
	}
	return nil
}

// mergeThreshold returns the effective node-count threshold from opts and totalNodes.
func mergeThreshold(opts MergeOptions, totalNodes int) int {
	threshold := opts.MinSize
	if frac := int(math.Ceil(opts.MinFraction * float64(totalNodes))); frac > threshold {
		threshold = frac
	}
	return threshold
}

// effectiveResolution returns opts.Resolution, defaulting to 1.0.
func effectiveResolution(opts MergeOptions) float64 {
	if opts.Resolution == 0 {
		return 1.0
	}
	return opts.Resolution
}

// candidate is a community eligible for merging.
type candidate struct {
	comm int
	size int
}

// MergeSmallCommunities post-processes a disjoint partition by absorbing
// communities that satisfy the MinSize or MinFraction threshold into their
// best neighbour according to Strategy. Community IDs in the returned result
// are re-numbered to be contiguous. Modularity is recomputed on the merged
// partition.
//
// Returns the input result unchanged when no merge threshold is set
// (MinSize==0 and MinFraction==0) or no communities qualify.
func MergeSmallCommunities(g *Graph, result CommunityResult, opts MergeOptions) (CommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return CommunityResult{}, err
	}

	// Validate partition nodes exist in graph.
	for n := range result.Partition {
		if _, ok := g.nodes[n]; !ok {
			return CommunityResult{}, ErrPartitionGraphMismatch
		}
	}

	threshold := mergeThreshold(opts, len(result.Partition))
	if threshold == 0 {
		return result, nil
	}

	// Build community → nodes map.
	commNodes := make(map[int][]NodeID)
	for n, c := range result.Partition {
		commNodes[c] = append(commNodes[c], n)
	}

	// Identify candidates (size < threshold), sorted ascending by size for
	// smallest-first processing.
	var candidates []candidate
	for c, nodes := range commNodes {
		if len(nodes) < threshold {
			candidates = append(candidates, candidate{c, len(nodes)})
		}
	}
	if len(candidates) == 0 {
		return result, nil
	}

	// Sort smallest-first (stable by comm ID for determinism).
	sortCandidates(candidates)

	// Work on a copy of the partition.
	partition := copyPartition(result.Partition)

	for _, cand := range candidates {
		nodes, ok := commNodes[cand.comm]
		if !ok {
			continue // already merged away
		}
		if len(nodes) >= threshold {
			continue // grew past threshold after absorbing an earlier candidate
		}
		target, found := findMergeTarget(g, nodes, cand.comm, partition, opts)
		if !found {
			continue // isolated community — leave in place
		}
		// Merge cand.comm → target.
		for _, n := range nodes {
			partition[n] = target
		}
		commNodes[target] = append(commNodes[target], nodes...)
		delete(commNodes, cand.comm)
	}

	// Renumber to contiguous 0-indexed IDs.
	newPartition := compactPartition(partition)
	return CommunityResult{
		Partition:  newPartition,
		Modularity: ComputeModularity(g, newPartition),
	}, nil
}

// copyPartition returns a shallow copy of partition.
func copyPartition(p map[NodeID]int) map[NodeID]int {
	out := make(map[NodeID]int, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

// compactPartition remaps community IDs to 0-indexed contiguous integers.
// The assignment of new IDs to surviving communities is non-deterministic
// (map iteration order), so callers must not rely on specific ID values.
func compactPartition(p map[NodeID]int) map[NodeID]int {
	remap := make(map[int]int)
	next := 0
	out := make(map[NodeID]int, len(p))
	for n, c := range p {
		if _, ok := remap[c]; !ok {
			remap[c] = next
			next++
		}
		out[n] = remap[c]
	}
	return out
}

// sortCandidates sorts candidates ascending by size, then comm ID for determinism.
func sortCandidates(cs []candidate) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && (cs[j].size < cs[j-1].size ||
			(cs[j].size == cs[j-1].size && cs[j].comm < cs[j-1].comm)); j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

// findMergeTarget returns the best community for the given nodes to merge into.
// Returns (0, false) if no neighbouring community exists (isolated community).
func findMergeTarget(g *Graph, nodes []NodeID, srcComm int, partition map[NodeID]int, opts MergeOptions) (int, bool) {
	// Collect neighbouring communities and their aggregate weight.
	neighborWeight := make(map[int]float64)
	for _, n := range nodes {
		for _, e := range g.Neighbors(n) {
			c, ok := partition[e.To]
			if !ok || c == srcComm {
				continue
			}
			neighborWeight[c] += e.Weight
		}
	}
	if len(neighborWeight) == 0 {
		return 0, false
	}

	switch opts.Strategy {
	case MergeByModularity:
		return bestByModularity(g, neighborWeight, nodes, partition, effectiveResolution(opts))
	default: // MergeByConnectivity
		return bestByConnectivity(neighborWeight)
	}
}

// bestByConnectivity returns the community with the highest aggregate edge weight.
func bestByConnectivity(neighborWeight map[int]float64) (int, bool) {
	best, bestW := -1, -1.0
	for c, w := range neighborWeight {
		if w > bestW || (w == bestW && c < best) {
			best, bestW = c, w
		}
	}
	return best, true
}

// bestByModularity returns the community whose merge yields the greatest ΔQ.
// ΔQ(src→t) = 2·w(src,t)/m  −  2·γ·s(src)·s(t)/m²
func bestByModularity(g *Graph, neighborWeight map[int]float64, srcNodes []NodeID, partition map[NodeID]int, gamma float64) (int, bool) {
	m := g.TotalWeight()
	if m == 0 {
		return bestByConnectivity(neighborWeight)
	}

	var srcStr float64
	for _, n := range srcNodes {
		srcStr += g.Strength(n)
	}

	best, bestDQ := -1, -1e18
	for t, w := range neighborWeight {
		tStr := g.CommStrength(t, partition)
		dq := 2*w/m - 2*gamma*srcStr*tStr/(m*m)
		if dq > bestDQ || (dq == bestDQ && t < best) {
			best, bestDQ = t, dq
		}
	}
	return best, true
}

// MergeSmallOverlappingCommunities post-processes an overlapping partition by
// absorbing communities below the threshold into the neighbour with the most
// shared-node overlap. NodeCommunities is rebuilt to be consistent with the
// merged Communities slice.
//
// The caller must ensure each Communities[i] slice contains no duplicate NodeIDs,
// as produced by EgoSplitting. Duplicate members will cause incorrect size
// calculations and union behaviour.
func MergeSmallOverlappingCommunities(g *Graph, result OverlappingCommunityResult, opts MergeOptions) (OverlappingCommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return OverlappingCommunityResult{}, err
	}

	totalNodes := len(result.NodeCommunities)
	threshold := mergeThreshold(opts, totalNodes)
	if threshold == 0 {
		return result, nil
	}

	// Work on copies — do not mutate the input.
	communities := make([][]NodeID, len(result.Communities))
	for i, c := range result.Communities {
		cp := make([]NodeID, len(c))
		copy(cp, c)
		communities[i] = cp
	}
	nodeCommunities := make(map[NodeID][]int, len(result.NodeCommunities))
	for n, cs := range result.NodeCommunities {
		cp := make([]int, len(cs))
		copy(cp, cs)
		nodeCommunities[n] = cp
	}

	// Identify candidates: non-nil communities with size < threshold.
	type ovCandidate struct{ idx, size int }
	var candidates []ovCandidate
	for i, c := range communities {
		if len(c) > 0 && len(c) < threshold {
			candidates = append(candidates, ovCandidate{i, len(c)})
		}
	}
	if len(candidates) == 0 {
		return result, nil
	}
	// Sort smallest-first, tie-break by idx.
	for i := 1; i < len(candidates); i++ {
		for j := i; j > 0 && (candidates[j].size < candidates[j-1].size ||
			(candidates[j].size == candidates[j-1].size && candidates[j].idx < candidates[j-1].idx)); j-- {
			candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
		}
	}

	for _, cand := range candidates {
		src := communities[cand.idx]
		if len(src) == 0 {
			continue // already consumed by an earlier merge
		}

		// Find neighbour community with most shared nodes (overlap-based).
		overlap := make(map[int]int)
		for _, n := range src {
			for _, ci := range nodeCommunities[n] {
				if ci != cand.idx && communities[ci] != nil {
					overlap[ci]++
				}
			}
		}

		var target int
		if len(overlap) > 0 {
			// Pick highest overlap; tie-break by idx.
			target = -1
			bestOverlap := -1
			for ci, cnt := range overlap {
				if cnt > bestOverlap || (cnt == bestOverlap && ci < target) {
					target, bestOverlap = ci, cnt
				}
			}
		} else {
			// No shared-membership neighbours — fall back to graph connectivity.
			connWeight := make(map[int]float64)
			for _, n := range src {
				for _, e := range g.Neighbors(n) {
					for _, ci := range nodeCommunities[e.To] {
						if ci != cand.idx && communities[ci] != nil {
							connWeight[ci] += e.Weight
						}
					}
				}
			}
			if len(connWeight) == 0 {
				continue // isolated — leave in place
			}
			target = -1
			bestW := -1.0
			for ci, w := range connWeight {
				if w > bestW || (w == bestW && ci < target) {
					target = ci
					bestW = w
				}
			}
		}

		// Union src into target (add only nodes not already in target).
		existing := make(map[NodeID]struct{}, len(communities[target]))
		for _, n := range communities[target] {
			existing[n] = struct{}{}
		}
		for _, n := range src {
			if _, ok := existing[n]; !ok {
				communities[target] = append(communities[target], n)
				nodeCommunities[n] = append(nodeCommunities[n], target)
			}
		}
		communities[cand.idx] = nil // mark as consumed
	}

	// Compact: remove nil slots and rebuild NodeCommunities with new indices.
	var compacted [][]NodeID
	remap := make(map[int]int)
	for i, c := range communities {
		if c != nil {
			remap[i] = len(compacted)
			compacted = append(compacted, c)
		}
	}

	newNodeComm := make(map[NodeID][]int, len(nodeCommunities))
	for n, cs := range nodeCommunities {
		seen := make(map[int]struct{})
		var newCS []int
		for _, ci := range cs {
			if newIdx, ok := remap[ci]; ok {
				if _, dup := seen[newIdx]; !dup {
					newCS = append(newCS, newIdx)
					seen[newIdx] = struct{}{}
				}
			}
		}
		if len(newCS) > 0 {
			newNodeComm[n] = newCS
		}
	}

	return OverlappingCommunityResult{
		Communities:     compacted,
		NodeCommunities: newNodeComm,
	}, nil
}
