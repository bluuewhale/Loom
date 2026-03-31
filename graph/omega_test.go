package graph

import (
	"testing"

	"github.com/bluuewhale/loom/graph/testdata"
)

// TestOmegaIndex_IdenticalPartitions verifies that OmegaIndex returns 1.0 when
// the detected result exactly matches the ground truth.
func TestOmegaIndex_IdenticalPartitions(t *testing.T) {
	// Build ground truth from KarateClubPartition (two-community split).
	gt := partitionToGroundTruth(testdata.KarateClubPartition)

	// Build an OverlappingCommunityResult that exactly mirrors the ground truth.
	result := groundTruthToResult(gt)

	omega := OmegaIndex(result, gt)
	if omega != 1.0 {
		t.Errorf("OmegaIndex for identical partitions = %v, want 1.0", omega)
	}
}

// TestOmegaIndex_CompletelyDifferent verifies that OmegaIndex returns a low value
// when the detected result and ground truth disagree substantially.
func TestOmegaIndex_CompletelyDifferent(t *testing.T) {
	// Ground truth: each node in its own singleton community.
	nodes := []NodeID{0, 1, 2, 3}
	gt := make([][]NodeID, len(nodes))
	for i, n := range nodes {
		gt[i] = []NodeID{n}
	}

	// Result: all nodes in one community.
	allComm := make([]NodeID, len(nodes))
	copy(allComm, nodes)
	result := OverlappingCommunityResult{
		Communities: [][]NodeID{allComm},
		NodeCommunities: map[NodeID][]int{
			0: {0},
			1: {0},
			2: {0},
			3: {0},
		},
	}

	omega := OmegaIndex(result, gt)
	if omega >= 0.5 {
		t.Errorf("OmegaIndex for completely different partitions = %v, want < 0.5", omega)
	}
}

// TestOmegaIndex_EmptyInput verifies that OmegaIndex returns 0.0 for empty inputs.
func TestOmegaIndex_EmptyInput(t *testing.T) {
	result := OverlappingCommunityResult{}
	gt := [][]NodeID{}
	omega := OmegaIndex(result, gt)
	if omega != 0.0 {
		t.Errorf("OmegaIndex for empty input = %v, want 0.0", omega)
	}
}

// TestOmegaIndex_TwoNodes verifies a minimal case: 2 nodes in the same community
// in both result and ground truth yields OmegaIndex == 1.0.
func TestOmegaIndex_TwoNodes(t *testing.T) {
	gt := [][]NodeID{{0, 1}}
	result := OverlappingCommunityResult{
		Communities:     [][]NodeID{{0, 1}},
		NodeCommunities: map[NodeID][]int{0: {0}, 1: {0}},
	}
	omega := OmegaIndex(result, gt)
	if omega != 1.0 {
		t.Errorf("OmegaIndex for two-node identical partition = %v, want 1.0", omega)
	}
}

// TestOmegaIndex_ReturnsInRange verifies that OmegaIndex always returns a value
// in [0, 1] for several constructed partition scenarios.
func TestOmegaIndex_ReturnsInRange(t *testing.T) {
	scenarios := []struct {
		name   string
		result OverlappingCommunityResult
		gt     [][]NodeID
	}{
		{
			name: "all in one vs all separate",
			result: OverlappingCommunityResult{
				Communities:     [][]NodeID{{0, 1, 2, 3}},
				NodeCommunities: map[NodeID][]int{0: {0}, 1: {0}, 2: {0}, 3: {0}},
			},
			gt: [][]NodeID{{0}, {1}, {2}, {3}},
		},
		{
			name: "overlapping result vs disjoint gt",
			result: OverlappingCommunityResult{
				Communities:     [][]NodeID{{0, 1, 2}, {1, 2, 3}},
				NodeCommunities: map[NodeID][]int{0: {0}, 1: {0, 1}, 2: {0, 1}, 3: {1}},
			},
			gt: [][]NodeID{{0, 1}, {2, 3}},
		},
		{
			name: "partial match",
			result: OverlappingCommunityResult{
				Communities:     [][]NodeID{{0, 1}, {2, 3}},
				NodeCommunities: map[NodeID][]int{0: {0}, 1: {0}, 2: {1}, 3: {1}},
			},
			gt: [][]NodeID{{0, 1, 2}, {3}},
		},
		{
			name:   "single node",
			result: OverlappingCommunityResult{},
			gt:     [][]NodeID{{5}},
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			omega := OmegaIndex(sc.result, sc.gt)
			if omega < 0.0 || omega > 1.0 {
				t.Errorf("OmegaIndex = %v, want in [0, 1]", omega)
			}
		})
	}
}

// --- helpers ---

// partitionToGroundTruth converts a map[int]int ground-truth partition (node -> community)
// into the [][]NodeID format expected by OmegaIndex.
func partitionToGroundTruth(partition map[int]int) [][]NodeID {
	groups := make(map[int][]NodeID)
	for node, comm := range partition {
		groups[comm] = append(groups[comm], NodeID(node))
	}
	// Determine number of communities.
	maxComm := -1
	for c := range groups {
		if c > maxComm {
			maxComm = c
		}
	}
	gt := make([][]NodeID, maxComm+1)
	for c, members := range groups {
		gt[c] = members
	}
	return gt
}

// groundTruthToResult converts a [][]NodeID ground truth into an
// OverlappingCommunityResult that matches it exactly.
func groundTruthToResult(gt [][]NodeID) OverlappingCommunityResult {
	communities := make([][]NodeID, len(gt))
	for i, members := range gt {
		cp := make([]NodeID, len(members))
		copy(cp, members)
		communities[i] = cp
	}

	nodeCommunities := make(map[NodeID][]int)
	for i, members := range gt {
		for _, node := range members {
			nodeCommunities[node] = append(nodeCommunities[node], i)
		}
	}

	return OverlappingCommunityResult{
		Communities:     communities,
		NodeCommunities: nodeCommunities,
	}
}
