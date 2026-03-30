package graph

import "errors"

// ErrNotImplemented is returned by Detect until the Ego Splitting algorithm is implemented.
var ErrNotImplemented = errors.New("ego splitting: not implemented")

// OverlappingCommunityDetector detects overlapping (non-disjoint) communities
// where a single node may belong to multiple communities.
// Implementations must be safe for concurrent use on distinct *Graph instances.
type OverlappingCommunityDetector interface {
	Detect(g *Graph) (OverlappingCommunityResult, error)
}

// OverlappingCommunityResult holds the output of an overlapping community detection run.
type OverlappingCommunityResult struct {
	Communities     [][]NodeID       // community-first: each element is a list of member NodeIDs
	NodeCommunities map[NodeID][]int // node-first: nodeID -> list of community indices (O(1) lookup)
}

// EgoSplittingOptions configures the Ego Splitting algorithm.
// Zero values apply documented defaults:
//   - LocalDetector nil -> Louvain with default options
//   - GlobalDetector nil -> Louvain with default options
//   - Resolution 0.0 -> 1.0
type EgoSplittingOptions struct {
	LocalDetector  CommunityDetector
	GlobalDetector CommunityDetector
	Resolution     float64
}

// egoSplittingDetector implements OverlappingCommunityDetector using the
// Ego Splitting framework (Epasto, Lattanzi, Paes Leme, 2017).
type egoSplittingDetector struct {
	opts EgoSplittingOptions
}

// NewEgoSplitting returns an OverlappingCommunityDetector that uses the
// Ego Splitting framework. Nil detectors default to Louvain with default options.
func NewEgoSplitting(opts EgoSplittingOptions) OverlappingCommunityDetector {
	if opts.LocalDetector == nil {
		opts.LocalDetector = NewLouvain(LouvainOptions{})
	}
	if opts.GlobalDetector == nil {
		opts.GlobalDetector = NewLouvain(LouvainOptions{})
	}
	if opts.Resolution == 0 {
		opts.Resolution = 1.0
	}
	return &egoSplittingDetector{opts: opts}
}

// Detect runs the Ego Splitting algorithm on g and returns overlapping communities.
// This is a stub that returns ErrNotImplemented until Phase 07-08 provides the implementation.
func (d *egoSplittingDetector) Detect(g *Graph) (OverlappingCommunityResult, error) {
	return OverlappingCommunityResult{}, ErrNotImplemented
}
