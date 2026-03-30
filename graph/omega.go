package graph

import "sort"

// OmegaIndex computes the Omega index (Collins & Dent 1988) between a detected
// overlapping community result and a ground-truth partition expressed as [][]NodeID.
// Returns a float64 in [0, 1]. Returns 0.0 if there are fewer than 2 distinct nodes.
//
// The Omega index is an adjusted pair-counting measure: for every unordered pair
// (u, v), it counts how many communities simultaneously contain both u and v in
// the detected result (tResult) and in the ground truth (tGT), then computes
// agreement adjusted for chance.
func OmegaIndex(result OverlappingCommunityResult, groundTruth [][]NodeID) float64 {
	// Collect all unique node IDs from both result and ground truth into a sorted slice.
	nodeSet := make(map[NodeID]struct{})
	for node := range result.NodeCommunities {
		nodeSet[node] = struct{}{}
	}
	for _, comm := range groundTruth {
		for _, node := range comm {
			nodeSet[node] = struct{}{}
		}
	}

	allNodes := make([]NodeID, 0, len(nodeSet))
	for node := range nodeSet {
		allNodes = append(allNodes, node)
	}
	sort.Slice(allNodes, func(i, j int) bool { return allNodes[i] < allNodes[j] })

	if len(allNodes) < 2 {
		return 0.0
	}

	// Precompute resultMembership: node -> set of community indices in result.
	resultMembership := make(map[NodeID]map[int]struct{}, len(allNodes))
	for i, comm := range result.Communities {
		for _, node := range comm {
			if resultMembership[node] == nil {
				resultMembership[node] = make(map[int]struct{})
			}
			resultMembership[node][i] = struct{}{}
		}
	}

	// Precompute gtMembership: node -> set of community indices in groundTruth.
	gtMembership := make(map[NodeID]map[int]struct{}, len(allNodes))
	for i, comm := range groundTruth {
		for _, node := range comm {
			if gtMembership[node] == nil {
				gtMembership[node] = make(map[int]struct{})
			}
			gtMembership[node][i] = struct{}{}
		}
	}

	// Iterate all unordered pairs (u, v) with u < v.
	// For each pair count shared community memberships in result and ground truth.
	freqResult := make(map[int]int)
	freqGT := make(map[int]int)
	agree := 0
	totalPairs := 0

	for i := 0; i < len(allNodes); i++ {
		for j := i + 1; j < len(allNodes); j++ {
			u, v := allNodes[i], allNodes[j]
			totalPairs++

			// tResult: number of result communities containing both u and v.
			tResult := countSharedMemberships(resultMembership[u], resultMembership[v])

			// tGT: number of ground-truth communities containing both u and v.
			tGT := countSharedMemberships(gtMembership[u], gtMembership[v])

			if tResult == tGT {
				agree++
			}
			freqResult[tResult]++
			freqGT[tGT]++
		}
	}

	if totalPairs == 0 {
		return 0.0
	}

	observed := float64(agree) / float64(totalPairs)

	// Expected agreement by chance: sum_k P(tResult=k) * P(tGT=k).
	expected := 0.0
	for k, rCount := range freqResult {
		if gCount, ok := freqGT[k]; ok {
			expected += float64(rCount) * float64(gCount)
		}
	}
	expected /= float64(totalPairs) * float64(totalPairs)

	if expected >= 1.0 {
		// Degenerate: both partitions are identical and trivial.
		return 1.0
	}

	omega := (observed - expected) / (1.0 - expected)

	// Clamp to [0, 1].
	if omega < 0.0 {
		return 0.0
	}
	if omega > 1.0 {
		return 1.0
	}
	return omega
}

// countSharedMemberships returns the number of keys present in both a and b.
func countSharedMemberships(a, b map[int]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// Iterate over the smaller set.
	small, large := a, b
	if len(a) > len(b) {
		small, large = b, a
	}
	count := 0
	for k := range small {
		if _, ok := large[k]; ok {
			count++
		}
	}
	return count
}
