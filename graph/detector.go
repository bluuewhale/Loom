package graph

import "errors"

// ErrDirectedNotSupported is returned when Detect is called on a directed graph.
// Directed community detection is deferred to v2.
var ErrDirectedNotSupported = errors.New("community detection on directed graphs is not supported")

// CommunityDetector is a swappable community detection algorithm.
// Implementations must be safe for concurrent use on distinct *Graph instances.
type CommunityDetector interface {
	Detect(g *Graph) (CommunityResult, error)
}

// CommunityResult holds the output of a community detection run.
type CommunityResult struct {
	Partition  map[NodeID]int // node -> community ID (0-indexed contiguous)
	Modularity float64        // modularity Q of the final partition
	Passes     int            // number of full passes executed
	Moves      int            // total node moves across all passes
}

// LouvainOptions configures the Louvain algorithm.
// Zero values apply documented defaults:
//   - Resolution 0.0 -> 1.0
//   - Seed 0 -> random seed (non-deterministic)
//   - MaxPasses 0 -> unlimited (converge via tolerance)
//   - Tolerance 0.0 -> 1e-7
type LouvainOptions struct {
	Resolution float64
	Seed       int64
	MaxPasses  int
	Tolerance  float64
}

// LeidenOptions configures the Leiden algorithm.
// Zero values apply documented defaults:
//   - Resolution 0.0 -> 1.0
//   - Seed 0 -> random seed (non-deterministic)
//   - MaxIterations 0 -> unlimited
//   - Tolerance 0.0 -> 1e-7
type LeidenOptions struct {
	Resolution    float64
	Seed          int64
	MaxIterations int
	Tolerance     float64
}

// louvainDetector implements CommunityDetector using the Louvain algorithm.
type louvainDetector struct {
	opts LouvainOptions
}

// NewLouvain returns a CommunityDetector that uses the Louvain algorithm.
func NewLouvain(opts LouvainOptions) CommunityDetector {
	return &louvainDetector{opts: opts}
}

// leidenDetector implements CommunityDetector using the Leiden algorithm.
type leidenDetector struct {
	opts LeidenOptions
}

// NewLeiden returns a CommunityDetector that uses the Leiden algorithm.
func NewLeiden(opts LeidenOptions) CommunityDetector {
	return &leidenDetector{opts: opts}
}
