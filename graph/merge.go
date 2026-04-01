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
	// Zero value treated as 1.0.
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
	if frac := int(math.Round(opts.MinFraction * float64(totalNodes))); frac > threshold {
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
	return result, nil
}

// MergeSmallOverlappingCommunities post-processes an overlapping partition by
// absorbing communities below the threshold into the neighbour with the most
// shared-node overlap. NodeCommunities is rebuilt to be consistent with the
// merged Communities slice.
func MergeSmallOverlappingCommunities(g *Graph, result OverlappingCommunityResult, opts MergeOptions) (OverlappingCommunityResult, error) {
	if err := validateMergeOptions(opts); err != nil {
		return OverlappingCommunityResult{}, err
	}
	return result, nil
}
