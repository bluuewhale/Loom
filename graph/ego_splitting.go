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
// Pipeline: directed-graph guard → buildPersonaGraph (Algorithms 1+2) →
// GlobalDetector.Detect on persona graph → mapPersonasToOriginal (Algorithm 3) →
// deduplicate and compact community indices → build result.
func (d *egoSplittingDetector) Detect(g *Graph) (OverlappingCommunityResult, error) {
	// Guard: directed graphs not supported.
	if g.IsDirected() {
		return OverlappingCommunityResult{}, ErrDirectedNotSupported
	}

	// Step 1: Build persona graph (Algorithms 1 + 2).
	personaGraph, _, inverseMap, err := buildPersonaGraph(g, d.opts.LocalDetector)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Step 2: Run global detector on persona graph.
	globalResult, err := d.opts.GlobalDetector.Detect(personaGraph)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Step 3: Map persona communities back to original nodes (Algorithm 3).
	nodeCommunities := mapPersonasToOriginal(globalResult.Partition, inverseMap)

	// Step 4: Deduplicate community IDs per node.
	// mapPersonasToOriginal can emit duplicate community IDs when multiple
	// personas of the same original node land in the same global community.
	for node, comms := range nodeCommunities {
		seen := make(map[int]struct{}, len(comms))
		unique := comms[:0]
		for _, c := range comms {
			if _, ok := seen[c]; !ok {
				seen[c] = struct{}{}
				unique = append(unique, c)
			}
		}
		nodeCommunities[node] = unique
	}

	// Step 5: Build Communities [][]NodeID from NodeCommunities.
	// First pass: find max community ID to size the slice.
	maxComm := -1
	for _, comms := range nodeCommunities {
		for _, c := range comms {
			if c > maxComm {
				maxComm = c
			}
		}
	}

	communities := make([][]NodeID, maxComm+1)
	for node, comms := range nodeCommunities {
		for _, c := range comms {
			communities[c] = append(communities[c], node)
		}
	}

	// Compact: remove empty community slots (global detector may produce sparse IDs).
	var filtered [][]NodeID
	commRemap := make(map[int]int) // old index -> new contiguous index
	for i, members := range communities {
		if len(members) > 0 {
			commRemap[i] = len(filtered)
			filtered = append(filtered, members)
		}
	}

	// Remap NodeCommunities indices to match compacted Communities slice.
	for node, comms := range nodeCommunities {
		remapped := make([]int, len(comms))
		for j, c := range comms {
			remapped[j] = commRemap[c]
		}
		nodeCommunities[node] = remapped
	}

	return OverlappingCommunityResult{
		Communities:     filtered,
		NodeCommunities: nodeCommunities,
	}, nil
}

// buildEgoNet returns the ego-net of node v: the subgraph induced by v's
// neighbors, excluding v itself. This implements Algorithm 1 of the Ego
// Splitting framework.
func buildEgoNet(g *Graph, v NodeID) *Graph {
	neighbors := g.Neighbors(v)
	nodeIDs := make([]NodeID, 0, len(neighbors))
	for _, e := range neighbors {
		nodeIDs = append(nodeIDs, e.To)
	}
	return g.Subgraph(nodeIDs)
}

// buildPersonaGraph constructs the persona graph from g using the given local
// detector. For each node v, it builds the ego-net, detects local communities,
// and creates one persona node per (v, community) pair. Edges are rewired
// bidirectionally per Algorithm 2 Section 2.2.
//
// Returns:
//   - personaGraph: new undirected graph with persona nodes and rewired edges
//   - personaOf: map[NodeID]map[int]NodeID -- original node -> community -> PersonaID
//   - inverseMap: map[NodeID]NodeID -- PersonaID -> original NodeID
func buildPersonaGraph(g *Graph, localDetector CommunityDetector) (*Graph, map[NodeID]map[int]NodeID, map[NodeID]NodeID, error) {
	// Step 1: find maxNodeID to set next persona ID above existing IDs
	var maxNodeID NodeID
	for _, id := range g.Nodes() {
		if id > maxNodeID {
			maxNodeID = id
		}
	}
	nextPersona := maxNodeID + 1

	personaOf := make(map[NodeID]map[int]NodeID)
	inverseMap := make(map[NodeID]NodeID)
	// partitions[v] holds the ego-net partition for v: neighbor -> community ID in G_v
	partitions := make(map[NodeID]map[NodeID]int)

	// Step 3: for each node v, build ego-net and assign persona nodes
	for _, v := range g.Nodes() {
		personaOf[v] = make(map[int]NodeID)

		egoNet := buildEgoNet(g, v)
		if egoNet.NodeCount() == 0 {
			// Isolated node or no neighbors: single persona with community 0
			personaOf[v][0] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
			partitions[v] = make(map[NodeID]int) // empty partition
			continue
		}

		result, err := localDetector.Detect(egoNet)
		if err != nil {
			return nil, nil, nil, err
		}

		// Store partition for cross-lookup during edge wiring
		partitions[v] = result.Partition

		// Collect unique communities and assign persona nodes
		commsSeen := make(map[int]struct{})
		for _, commID := range result.Partition {
			commsSeen[commID] = struct{}{}
		}
		for commID := range commsSeen {
			personaOf[v][commID] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
		}
	}

	// Step 4-6: build persona graph and wire edges
	personaGraph := NewGraph(false)

	// Add all persona nodes
	for personaID := range inverseMap {
		personaGraph.AddNode(personaID, 1.0)
	}

	// Wire edges with dedup using canonical (lo, hi) key
	seen := make(map[[2]NodeID]struct{})
	for _, u := range g.Nodes() {
		for _, e := range g.Neighbors(u) {
			v := e.To

			// Canonical dedup: process each undirected edge once
			lo, hi := u, v
			if lo > hi {
				lo, hi = hi, lo
			}
			key := [2]NodeID{lo, hi}
			if _, already := seen[key]; already {
				continue
			}
			seen[key] = struct{}{}

			// Determine which persona of u handles this edge:
			// u's persona is determined by the community of v in G_u (u's ego-net).
			// If v is absent from G_u's partition (e.g. v not in G_u), fall back to
			// community 0 — u has a single persona for isolated neighbors.
			commOfVinGu := 0
			if partU, hasU := partitions[u]; hasU {
				if cv, vInU := partU[v]; vInU {
					commOfVinGu = cv
				}
			}

			// Determine which persona of v handles this edge:
			// v's persona is determined by the community of u in G_v (v's ego-net).
			// Same fallback for u absent from G_v's partition.
			commOfUinGv := 0
			if partV, hasV := partitions[v]; hasV {
				if cu, uInV := partV[u]; uInV {
					commOfUinGv = cu
				}
			}

			// Look up persona nodes
			personaU, uHasComm := personaOf[u][commOfVinGu]
			if !uHasComm {
				continue
			}
			personaV, vHasComm := personaOf[v][commOfUinGv]
			if !vHasComm {
				continue
			}

			personaGraph.AddEdge(personaU, personaV, e.Weight)
		}
	}

	return personaGraph, personaOf, inverseMap, nil
}

// mapPersonasToOriginal converts global community assignments on the persona
// graph back to overlapping community memberships on the original graph.
// This implements Algorithm 3 of the Ego Splitting framework.
func mapPersonasToOriginal(
	globalPartition map[NodeID]int,
	inverseMap map[NodeID]NodeID,
) map[NodeID][]int {
	result := make(map[NodeID][]int)
	for personaID, commID := range globalPartition {
		origNode := inverseMap[personaID]
		result[origNode] = append(result[origNode], commID)
	}
	return result
}
