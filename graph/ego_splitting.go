package graph

import (
	"errors"
	"runtime"
	"sync"
)

// ErrEmptyGraph is returned when Detect is called on a graph with no nodes.
var ErrEmptyGraph = errors.New("ego splitting: empty graph")

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

	// Unexported carry-forward fields populated by Detect() for use by Update().
	// These hold the intermediate state needed for incremental recomputation.
	personaOf        map[NodeID]map[int]NodeID // original node -> community -> PersonaID
	inverseMap       map[NodeID]NodeID         // PersonaID -> original NodeID
	partitions       map[NodeID]map[NodeID]int // ego-net partition per original node
	personaPartition map[NodeID]int            // persona-level global partition
	personaGraph     *Graph                    // last-built persona graph (for Clone fast-path)
}

// EgoSplittingOptions configures the Ego Splitting algorithm.
// Zero values apply documented defaults:
//   - LocalDetector nil -> Louvain with default options
//   - GlobalDetector nil -> Louvain with default options
//   - Resolution 0.0 -> 1.0
//
// Note: Resolution is stored for reference but is not automatically propagated
// to the default-constructed Louvain detectors. To use a custom resolution,
// supply a pre-configured detector via LocalDetector and/or GlobalDetector.
type EgoSplittingOptions struct {
	LocalDetector  CommunityDetector
	GlobalDetector CommunityDetector
	Resolution     float64
}

// DeltaEdge represents a directed or undirected edge addition in a GraphDelta.
// Unlike Edge (which is always relative to a source node), DeltaEdge carries
// both endpoints so it can stand alone in a delta description.
type DeltaEdge struct {
	From   NodeID
	To     NodeID
	Weight float64
}

// GraphDelta describes incremental additions to a graph for use with Update().
// Only additions are supported in v1.3; deletions are deferred to v1.4.
type GraphDelta struct {
	AddedNodes []NodeID
	AddedEdges []DeltaEdge
}

// OnlineOverlappingCommunityDetector extends OverlappingCommunityDetector with
// warm-start and incremental Update support for append-only graph mutations.
type OnlineOverlappingCommunityDetector interface {
	OverlappingCommunityDetector
	// DetectWithPrior runs a full rebuild of the persona graph but warm-starts
	// the GlobalDetector using priorNodeCommunities (NodeID -> community indices).
	// Use this when you have a prior community assignment from a previous run
	// (e.g. on a similar graph) and want to bias detection toward it.
	// For incremental updates to the same graph, prefer Update().
	DetectWithPrior(g *Graph, priorNodeCommunities map[NodeID][]int) (OverlappingCommunityResult, error)
	Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)
}

// egoSplittingDetector implements OverlappingCommunityDetector using the
// Ego Splitting framework (Epasto, Lattanzi, Paes Leme, 2017).
type egoSplittingDetector struct {
	opts EgoSplittingOptions
}

// NewEgoSplitting returns an OverlappingCommunityDetector that uses the
// Ego Splitting framework. Nil detectors default to Louvain with default options.
// The default GlobalDetector uses MaxPasses=1: the persona graph is typically
// sparse (avg degree ≈ 1) so a single Louvain pass finds near-optimal communities
// without the O(n log n) supergraph compression of later passes.
func NewEgoSplitting(opts EgoSplittingOptions) OverlappingCommunityDetector {
	if opts.LocalDetector == nil {
		opts.LocalDetector = NewLouvain(LouvainOptions{})
	}
	if opts.GlobalDetector == nil {
		opts.GlobalDetector = NewLouvain(LouvainOptions{MaxPasses: 1})
	}
	if opts.Resolution == 0 {
		opts.Resolution = 1.0
	}
	return &egoSplittingDetector{opts: opts}
}

// NewOnlineEgoSplitting returns an OnlineOverlappingCommunityDetector backed by
// the Ego Splitting algorithm. Nil detectors default to Louvain with default options.
// The default GlobalDetector uses MaxPasses=1 for the same reason as NewEgoSplitting.
func NewOnlineEgoSplitting(opts EgoSplittingOptions) OnlineOverlappingCommunityDetector {
	if opts.LocalDetector == nil {
		opts.LocalDetector = NewLouvain(LouvainOptions{})
	}
	if opts.GlobalDetector == nil {
		opts.GlobalDetector = NewLouvain(LouvainOptions{MaxPasses: 1})
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
	// Guard: empty graph not supported.
	if g.NodeCount() == 0 {
		return OverlappingCommunityResult{}, ErrEmptyGraph
	}

	// Step 1: Build persona graph (Algorithms 1 + 2).
	personaGraph, personaOf, inverseMap, partitions, err := buildPersonaGraph(g, d.opts.LocalDetector)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Step 2: Run global detector on persona graph.
	globalResult, err := d.opts.GlobalDetector.Detect(personaGraph)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Steps 3-5: map personas back, deduplicate, and compact.
	filtered, nodeCommunities := compactCommunities(globalResult.Partition, inverseMap)

	return OverlappingCommunityResult{
		Communities:      filtered,
		NodeCommunities:  nodeCommunities,
		personaOf:        personaOf,
		inverseMap:       inverseMap,
		partitions:       partitions,
		personaPartition: globalResult.Partition,
		personaGraph:     personaGraph,
	}, nil
}

// DetectWithPrior runs the Ego Splitting algorithm on g with a warm-started
// GlobalDetector. The persona graph is rebuilt from scratch (full rebuild),
// but the GlobalDetector receives an initial partition derived from
// priorNodeCommunities (original NodeID → list of prior community indices).
//
// Warm-start projection: for each original node v with personas keyed by
// local-community index 0, 1, …, k-1, persona for local community i is
// assigned priorNodeCommunities[v][i % len(prior)]. Nodes absent from
// priorNodeCommunities receive fresh singleton community IDs.
//
// If priorNodeCommunities is nil or empty, falls back to Detect().
func (d *egoSplittingDetector) DetectWithPrior(
	g *Graph,
	priorNodeCommunities map[NodeID][]int,
) (OverlappingCommunityResult, error) {
	if g.IsDirected() {
		return OverlappingCommunityResult{}, ErrDirectedNotSupported
	}
	if g.NodeCount() == 0 {
		return OverlappingCommunityResult{}, ErrEmptyGraph
	}
	if len(priorNodeCommunities) == 0 {
		return d.Detect(g)
	}

	// Step 1: Full persona graph rebuild (same as Detect).
	personaGraph, personaOf, inverseMap, partitions, err := buildPersonaGraph(g, d.opts.LocalDetector)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Step 2: Project priorNodeCommunities onto persona nodes to build a
	// warm-start partition for GlobalDetector.
	//
	// For each original node v, iterate its personas (keyed by local community
	// index). Assign persona i the prior community priorNodeCommunities[v][i%m].
	warmPartition := make(map[NodeID]int, len(inverseMap))
	for v, commPersonas := range personaOf {
		priorComms, hasPrior := priorNodeCommunities[v]
		if !hasPrior || len(priorComms) == 0 {
			continue // handled below as singletons
		}
		for localComm, personaID := range commPersonas {
			warmPartition[personaID] = priorComms[localComm%len(priorComms)]
		}
	}
	// Assign fresh singleton IDs to personas with no prior coverage.
	maxCommID := -1
	for _, c := range warmPartition {
		if c > maxCommID {
			maxCommID = c
		}
	}
	for _, commPersonas := range personaOf {
		for _, personaID := range commPersonas {
			if _, assigned := warmPartition[personaID]; !assigned {
				maxCommID++
				warmPartition[personaID] = maxCommID
			}
		}
	}

	// Step 3: Run warm-started GlobalDetector on persona graph.
	warmGlobal := warmStartedDetector(d.opts.GlobalDetector, warmPartition)
	globalResult, err := warmGlobal.Detect(personaGraph)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Steps 4–5: map back, deduplicate, and compact.
	filtered, nodeCommunities := compactCommunities(globalResult.Partition, inverseMap)

	return OverlappingCommunityResult{
		Communities:      filtered,
		NodeCommunities:  nodeCommunities,
		personaOf:        personaOf,
		inverseMap:       inverseMap,
		partitions:       partitions,
		personaPartition: globalResult.Partition,
		personaGraph:     personaGraph,
	}, nil
}

// Update returns an updated overlapping community result incorporating delta.
// If delta is empty (no added nodes or edges), prior is returned unchanged with
// zero additional allocations. Returns ErrDirectedNotSupported if g is directed.
//
// For non-empty deltas, Update performs incremental recomputation:
//   - Only affected nodes' ego-nets are recomputed (ONLINE-05)
//   - Unaffected nodes' PersonaIDs carry over from prior (ONLINE-06)
//   - New PersonaIDs are allocated above max(prior, g.Nodes()) (ONLINE-11)
//   - Global detection is warm-started from prior persona partition (ONLINE-07)
//
// If prior lacks carry-forward fields (nil personaOf), falls back to Detect().
func (d *egoSplittingDetector) Update(
	g *Graph,
	delta GraphDelta,
	prior OverlappingCommunityResult,
) (OverlappingCommunityResult, error) {
	if g.IsDirected() {
		return OverlappingCommunityResult{}, ErrDirectedNotSupported
	}
	if len(delta.AddedNodes) == 0 && len(delta.AddedEdges) == 0 {
		return prior, nil
	}
	// Graceful fallback if prior lacks carry-forward fields.
	if prior.personaOf == nil || prior.inverseMap == nil || prior.partitions == nil {
		return d.Detect(g)
	}

	// Step 1: Compute affected node set (ONLINE-05).
	affected := computeAffected(g, delta)

	// Step 2: Incremental persona graph build (ONLINE-06, ONLINE-11).
	personaGraph, newPersonaOf, newInverseMap, newPartitions, warmPartition, isolatedOnly, err :=
		buildPersonaGraphIncremental(g, affected, prior, d.opts.LocalDetector)
	if err != nil {
		return OverlappingCommunityResult{}, err
	}

	// Step 3: Global detection (ONLINE-07).
	// Fast-path: when all new personas are isolated (no edges in persona graph),
	// they can only be singleton communities. Extend warmPartition with new persona
	// singletons and skip the global Louvain run entirely.
	var globalPartition map[NodeID]int
	if isolatedOnly {
		// Find the maximum existing community ID to assign new singletons beyond it.
		maxCommID := -1
		for _, c := range warmPartition {
			if c > maxCommID {
				maxCommID = c
			}
		}
		globalPartition = make(map[NodeID]int, len(warmPartition)+len(newInverseMap)-len(warmPartition))
		for pID, c := range warmPartition {
			globalPartition[pID] = c
		}
		// Assign each new persona its own singleton community.
		for pID := range newInverseMap {
			if _, alreadyAssigned := globalPartition[pID]; !alreadyAssigned {
				maxCommID++
				globalPartition[pID] = maxCommID
			}
		}
	} else {
		warmGlobal := warmStartedDetector(d.opts.GlobalDetector, warmPartition)
		var globalResult CommunityResult
		globalResult, err = warmGlobal.Detect(personaGraph)
		if err != nil {
			return OverlappingCommunityResult{}, err
		}
		globalPartition = globalResult.Partition
	}

	// Steps 4-5: map personas back, deduplicate, and compact.
	filtered, nodeCommunities := compactCommunities(globalPartition, newInverseMap)

	return OverlappingCommunityResult{
		Communities:      filtered,
		NodeCommunities:  nodeCommunities,
		personaOf:        newPersonaOf,
		inverseMap:       newInverseMap,
		partitions:       newPartitions,
		personaPartition: globalPartition,
		personaGraph:     personaGraph,
	}, nil
}

// warmStartedDetector constructs a new CommunityDetector with the same
// configuration as d but with InitialPartition set to partition.
// Does NOT mutate d. Falls back to d unchanged if type is unrecognized.
func warmStartedDetector(d CommunityDetector, partition map[NodeID]int) CommunityDetector {
	switch det := d.(type) {
	case *louvainDetector:
		return NewLouvain(LouvainOptions{
			Resolution:       det.opts.Resolution,
			Seed:             det.opts.Seed,
			MaxPasses:        det.opts.MaxPasses,
			Tolerance:        det.opts.Tolerance,
			InitialPartition: partition,
		})
	case *leidenDetector:
		return NewLeiden(LeidenOptions{
			Resolution:       det.opts.Resolution,
			Seed:             det.opts.Seed,
			MaxIterations:    det.opts.MaxIterations,
			Tolerance:        det.opts.Tolerance,
			InitialPartition: partition,
		})
	default:
		return d
	}
}

// cloneDetector returns a fresh CommunityDetector with the same configuration
// as d but with no shared mutable state (e.g. RNG, sync.Pool entries).
// This is required so that goroutine workers each have an independent detector
// instance — CommunityDetectors are NOT safe to call concurrently on the same
// instance. Falls back to d itself for unknown types (safe if caller ensures
// serial use, but goroutine workers should never reach the default branch
// in normal operation).
func cloneDetector(d CommunityDetector) CommunityDetector {
	switch det := d.(type) {
	case *louvainDetector:
		opts := det.opts
		opts.InitialPartition = nil // clones must start cold
		return NewLouvain(opts)
	case *leidenDetector:
		opts := det.opts
		opts.InitialPartition = nil
		return NewLeiden(opts)
	default:
		return d
	}
}

// egoNetJob is a unit of work sent to parallel ego-net detection workers.
type egoNetJob struct {
	v      NodeID
	egoNet *Graph
}

// egoNetResult holds the output of one ego-net detection job.
type egoNetResult struct {
	v         NodeID
	partition map[NodeID]int
	err       error
}

// runParallelEgoNets dispatches ego-net detection jobs across a bounded worker
// pool and collects results. jobs must be closed by the caller after all sends.
// workerCount controls pool size; each worker gets its own cloneDetector(det) copy.
func runParallelEgoNets(jobs <-chan egoNetJob, det CommunityDetector, workerCount int) []egoNetResult {
	results := make(chan egoNetResult, workerCount*2)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(localDet CommunityDetector) {
			defer wg.Done()
			for job := range jobs {
				res, err := localDet.Detect(job.egoNet)
				results <- egoNetResult{v: job.v, partition: res.Partition, err: err}
			}
		}(cloneDetector(det))
	}
	// Close results channel after all workers finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	var out []egoNetResult
	for r := range results {
		out = append(out, r)
	}
	return out
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
//   - partitions: map[NodeID]map[NodeID]int -- ego-net partition per original node
func buildPersonaGraph(g *Graph, localDetector CommunityDetector) (*Graph, map[NodeID]map[int]NodeID, map[NodeID]NodeID, map[NodeID]map[NodeID]int, error) {
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

	// Step 2: build ego-nets and detect local communities in parallel.
	// Isolated nodes (no neighbors) are handled inline without goroutine overhead.
	// Non-empty ego-nets are dispatched to a bounded worker pool.
	nodes := g.Nodes()
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > len(nodes) {
		workerCount = len(nodes)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	jobCh := make(chan egoNetJob, workerCount*2)

	// Collect isolated nodes inline; store non-empty ego-nets for dispatch.
	// Ego-nets are built once here — not rebuilt in the goroutine.
	type nodeEgo struct {
		v      NodeID
		egoNet *Graph
	}
	var nonEmptyJobs []nodeEgo
	for _, v := range nodes {
		personaOf[v] = make(map[int]NodeID)
		egoNet := buildEgoNet(g, v)
		if egoNet.NodeCount() == 0 {
			// Isolated node: single persona, community 0, no detection needed.
			personaOf[v][0] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
			partitions[v] = make(map[NodeID]int)
			continue
		}
		nonEmptyJobs = append(nonEmptyJobs, nodeEgo{v, egoNet})
	}

	// Dispatch stored ego-nets to the worker pool (no rebuild).
	go func() {
		for _, job := range nonEmptyJobs {
			jobCh <- egoNetJob{v: job.v, egoNet: job.egoNet}
		}
		close(jobCh)
	}()

	results := runParallelEgoNets(jobCh, localDetector, workerCount)

	// Check for errors first.
	for _, r := range results {
		if r.err != nil {
			return nil, nil, nil, nil, r.err
		}
	}

	// Assign persona nodes for non-empty ego-net results.
	for _, r := range results {
		v := r.v
		partitions[v] = r.partition

		commsSeen := make(map[int]struct{})
		for _, commID := range r.partition {
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

	return personaGraph, personaOf, inverseMap, partitions, nil
}

// computeAffected returns the set of nodes whose ego-nets must be recomputed
// for the given delta. Affected = new nodes + neighbors of all edge endpoints
// (queried on the already-updated graph g).
func computeAffected(g *Graph, delta GraphDelta) map[NodeID]struct{} {
	affected := make(map[NodeID]struct{})
	for _, n := range delta.AddedNodes {
		affected[n] = struct{}{}
	}
	for _, e := range delta.AddedEdges {
		affected[e.From] = struct{}{}
		affected[e.To] = struct{}{}
		for _, nb := range g.Neighbors(e.From) {
			affected[nb.To] = struct{}{}
		}
		for _, nb := range g.Neighbors(e.To) {
			affected[nb.To] = struct{}{}
		}
	}
	return affected
}

// buildPersonaGraphIncremental constructs an updated persona graph using the
// prior result and recomputing only the affected nodes' ego-nets (ONLINE-06).
// New PersonaIDs are allocated above max(prior inverseMap keys, g.Nodes()) to
// prevent collisions (ONLINE-11).
//
// Fast-path: when all affected nodes are newly-added isolated nodes (no edges),
// the prior persona graph edges are unchanged. The prior graph is cloned and
// new persona nodes are appended — avoiding the full O(|E|) edge-wiring rebuild.
// In this case isolatedOnly=true is returned, signalling to the caller that
// global detection can be skipped (new personas are disconnected singletons).
//
// Returns: personaGraph, personaOf, inverseMap, partitions, warmPartition, isolatedOnly, error.
func buildPersonaGraphIncremental(
	g *Graph,
	affected map[NodeID]struct{},
	prior OverlappingCommunityResult,
	localDetector CommunityDetector,
) (*Graph, map[NodeID]map[int]NodeID, map[NodeID]NodeID, map[NodeID]map[NodeID]int, map[NodeID]int, bool, error) {
	// Step a: deep-copy prior maps (shallow copy of inner maps for unaffected nodes).
	personaOf := make(map[NodeID]map[int]NodeID, len(prior.personaOf))
	for v, comms := range prior.personaOf {
		personaOf[v] = comms // unaffected entries share the inner map (read-only)
	}
	inverseMap := make(map[NodeID]NodeID, len(prior.inverseMap))
	for pID, orig := range prior.inverseMap {
		inverseMap[pID] = orig
	}
	partitions := make(map[NodeID]map[NodeID]int, len(prior.partitions))
	for v, part := range prior.partitions {
		partitions[v] = part // unaffected entries share the inner map (read-only)
	}

	// Step b: compute nextPersona = max(all prior inverseMap keys, all g.Nodes()) + 1.
	// ONLINE-11: must consider both to prevent collisions after multiple Updates.
	var maxID NodeID
	for pID := range prior.inverseMap {
		if pID > maxID {
			maxID = pID
		}
	}
	for _, id := range g.Nodes() {
		if id > maxID {
			maxID = id
		}
	}
	nextPersona := maxID + 1

	// Step c: delete old persona entries for affected nodes.
	for v := range affected {
		if comms, ok := personaOf[v]; ok {
			for _, personaID := range comms {
				delete(inverseMap, personaID)
			}
		}
		delete(personaOf, v)
		delete(partitions, v)
	}

	// Fast-path: if all affected nodes are newly-added isolated nodes, the prior
	// persona graph edges are unchanged. Clone the prior graph and add new persona
	// nodes (all isolated — no edges) instead of rebuilding O(|E|) edges.
	// In this case no prior entries need deletion, so we can share prior maps directly.
	// Conditions: (1) prior personaGraph is stored, (2) every affected node is new
	// (not in prior.personaOf), (3) all have empty ego-nets in g.
	if prior.personaGraph != nil {
		allIsolatedNew := true
		for v := range affected {
			if _, wasPrior := prior.personaOf[v]; wasPrior {
				allIsolatedNew = false
				break
			}
			if len(g.Neighbors(v)) > 0 {
				allIsolatedNew = false
				break
			}
		}
		if allIsolatedNew {
			// Shallow-copy the outer maps (inner maps are shared read-only).
			newPersonaOf := make(map[NodeID]map[int]NodeID, len(prior.personaOf)+len(affected))
			for k, v := range prior.personaOf {
				newPersonaOf[k] = v
			}
			newInverseMap := make(map[NodeID]NodeID, len(prior.inverseMap)+len(affected))
			for k, v := range prior.inverseMap {
				newInverseMap[k] = v
			}
			newPartitions := make(map[NodeID]map[NodeID]int, len(prior.partitions)+len(affected))
			for k, v := range prior.partitions {
				newPartitions[k] = v
			}

			for v := range affected {
				m := make(map[int]NodeID, 1)
				m[0] = nextPersona
				newPersonaOf[v] = m
				newInverseMap[nextPersona] = v
				nextPersona++
				newPartitions[v] = make(map[NodeID]int)
			}
			// warmPartition = prior persona partition (all prior personas survive).
			warmPartition := prior.personaPartition

			// Clone prior persona graph and append new isolated persona nodes.
			pg := prior.personaGraph.Clone()
			for v := range affected {
				for _, personaID := range newPersonaOf[v] {
					pg.AddNode(personaID, 1.0)
				}
			}
			return pg, newPersonaOf, newInverseMap, newPartitions, warmPartition, true, nil
		}
	}

	// Step d: rebuild ego-nets for affected nodes in parallel.
	// Isolated nodes handled inline; non-empty ego-nets dispatched to worker pool.
	workerCount := runtime.GOMAXPROCS(0)
	if workerCount > len(affected) {
		workerCount = len(affected)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	// Initialize personaOf entries for all affected nodes (required before parallel dispatch).
	for v := range affected {
		personaOf[v] = make(map[int]NodeID)
	}

	// Separate isolated from non-empty affected nodes.
	// Build ego-nets once; reuse in goroutine dispatch (no double build).
	type affectedEgo struct {
		v      NodeID
		egoNet *Graph
	}
	var nonEmptyAffected []affectedEgo
	for v := range affected {
		egoNet := buildEgoNet(g, v)
		if egoNet.NodeCount() == 0 {
			personaOf[v][0] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
			partitions[v] = make(map[NodeID]int)
			continue
		}
		nonEmptyAffected = append(nonEmptyAffected, affectedEgo{v, egoNet})
	}

	if len(nonEmptyAffected) > 0 {
		jobCh := make(chan egoNetJob, workerCount*2)
		go func() {
			for _, job := range nonEmptyAffected {
				jobCh <- egoNetJob{v: job.v, egoNet: job.egoNet}
			}
			close(jobCh)
		}()

		results := runParallelEgoNets(jobCh, localDetector, workerCount)

		for _, r := range results {
			if r.err != nil {
				return nil, nil, nil, nil, nil, false, r.err
			}
		}

		for _, r := range results {
			v := r.v
			partitions[v] = r.partition

			commsSeen := make(map[int]struct{})
			for _, commID := range r.partition {
				commsSeen[commID] = struct{}{}
			}
			for commID := range commsSeen {
				personaOf[v][commID] = nextPersona
				inverseMap[nextPersona] = v
				nextPersona++
			}
		}
	}

	// Step e: build warmPartition from prior.personaPartition, keeping only
	// PersonaIDs that still exist in the new inverseMap (deleted affected
	// personas are excluded; new personas are absent = cold-start singleton).
	warmPartition := make(map[NodeID]int, len(prior.personaPartition))
	for pID, commID := range prior.personaPartition {
		if _, stillExists := inverseMap[pID]; stillExists {
			warmPartition[pID] = commID
		}
	}

	// Step f: incremental-patch or full rebuild.
	//
	// Incremental patch (fast-path when prior.personaGraph is available):
	//   1. Clone the prior persona graph (preserves all unaffected edges).
	//   2. Add new persona nodes for affected nodes.
	//   3. Remove all edges incident to affected personas (stale wiring).
	//   4. Re-wire only edges where at least one original endpoint is affected.
	// This avoids iterating all O(|E|) original edges — only affected-node edges are wired.
	//
	// Full rebuild (fallback): used when no prior persona graph is available.
	var personaGraph *Graph
	if prior.personaGraph != nil {
		personaGraph = prior.personaGraph.Clone()

		// Add new persona nodes (affected nodes got new PersonaIDs in step d).
		for v := range affected {
			for _, personaID := range personaOf[v] {
				personaGraph.AddNode(personaID, 1.0)
			}
		}

		// Collect the set of all persona IDs belonging to affected original nodes.
		affectedPersonas := make(map[NodeID]struct{})
		for v := range affected {
			for _, personaID := range personaOf[v] {
				affectedPersonas[personaID] = struct{}{}
			}
		}

		// Remove stale edges: any edge touching an affected persona is stale.
		personaGraph.RemoveEdgesFor(affectedPersonas)

		// Re-wire edges where at least one original endpoint is affected.
		// Only iterate edges incident to affected original nodes.
		seen := make(map[[2]NodeID]struct{})
		for u := range affected {
			for _, e := range g.Neighbors(u) {
				v := e.To
				lo, hi := u, v
				if lo > hi {
					lo, hi = hi, lo
				}
				key := [2]NodeID{lo, hi}
				if _, already := seen[key]; already {
					continue
				}
				seen[key] = struct{}{}

				commOfVinGu := 0
				if partU, hasU := partitions[u]; hasU {
					if cv, vInU := partU[v]; vInU {
						commOfVinGu = cv
					}
				}
				commOfUinGv := 0
				if partV, hasV := partitions[v]; hasV {
					if cu, uInV := partV[u]; uInV {
						commOfUinGv = cu
					}
				}

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
	} else {
		// Full rebuild: no prior persona graph available.
		personaGraph = NewGraph(false)
		for personaID := range inverseMap {
			personaGraph.AddNode(personaID, 1.0)
		}

		seen := make(map[[2]NodeID]struct{})
		for _, u := range g.Nodes() {
			for _, e := range g.Neighbors(u) {
				v := e.To
				lo, hi := u, v
				if lo > hi {
					lo, hi = hi, lo
				}
				key := [2]NodeID{lo, hi}
				if _, already := seen[key]; already {
					continue
				}
				seen[key] = struct{}{}

				commOfVinGu := 0
				if partU, hasU := partitions[u]; hasU {
					if cv, vInU := partU[v]; vInU {
						commOfVinGu = cv
					}
				}
				commOfUinGv := 0
				if partV, hasV := partitions[v]; hasV {
					if cu, uInV := partV[u]; uInV {
						commOfUinGv = cu
					}
				}

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
	}

	return personaGraph, personaOf, inverseMap, partitions, warmPartition, false, nil
}

// compactCommunities converts a persona-level global partition into the
// deduplicated, compacted Communities and NodeCommunities fields used in
// OverlappingCommunityResult. It is the shared implementation of Steps 4-5
// across Detect, DetectWithPrior, and Update.
func compactCommunities(
	globalPartition map[NodeID]int,
	inverseMap map[NodeID]NodeID,
) ([][]NodeID, map[NodeID][]int) {
	nodeCommunities := mapPersonasToOriginal(globalPartition, inverseMap)

	// Deduplicate community IDs per node.
	// mapPersonasToOriginal can emit duplicate IDs when multiple personas of the
	// same original node land in the same global community.
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

	// Build Communities [][]NodeID: find max ID, populate, then compact.
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
	var filtered [][]NodeID
	commRemap := make(map[int]int)
	for i, members := range communities {
		if len(members) > 0 {
			commRemap[i] = len(filtered)
			filtered = append(filtered, members)
		}
	}
	for node, comms := range nodeCommunities {
		remapped := make([]int, len(comms))
		for j, c := range comms {
			remapped[j] = commRemap[c]
		}
		nodeCommunities[node] = remapped
	}
	return filtered, nodeCommunities
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
		origNode, ok := inverseMap[personaID]
		if !ok {
			continue
		}
		result[origNode] = append(result[origNode], commID)
	}
	return result
}
