package graph

import (
	"errors"
	"runtime"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
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
// Warm-start projection: for each original node v, the local-community keys
// produced by LocalDetector are sorted and mapped positionally onto
// priorNodeCommunities[v] via modulo (key[i] → prior[i % len(prior)]). Because
// LocalDetector community IDs are opaque integers that may not be 0-based or
// contiguous, the mapping is performed on the sorted key order to ensure
// determinism. Nodes absent from priorNodeCommunities receive fresh singleton
// community IDs.
//
// priorNodeCommunities should originate from a previous detector run on a
// structurally identical or very similar graph (e.g. after incremental node
// additions). Using a prior from a structurally different graph may degrade
// rather than improve GlobalDetector convergence, because community IDs are
// graph-run-specific integers with no cross-graph meaning.
//
// If priorNodeCommunities is nil or empty, falls back to Detect().
func (d *egoSplittingDetector) DetectWithPrior(
	g *Graph,
	priorNodeCommunities map[NodeID][]int,
) (OverlappingCommunityResult, error) {
	if len(priorNodeCommunities) == 0 {
		return d.Detect(g)
	}
	if g.IsDirected() {
		return OverlappingCommunityResult{}, ErrDirectedNotSupported
	}
	if g.NodeCount() == 0 {
		return OverlappingCommunityResult{}, ErrEmptyGraph
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
		// Sort local-community keys for deterministic positional mapping;
		// LocalDetector IDs are opaque integers that may be non-contiguous.
		localKeys := make([]int, 0, len(commPersonas))
		for lc := range commPersonas {
			localKeys = append(localKeys, lc)
		}
		sort.Ints(localKeys)
		for i, lc := range localKeys {
			warmPartition[commPersonas[lc]] = priorComms[i%len(priorComms)]
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

// runParallelEgoNets dispatches ego-net detection across workerCount goroutines.
// An atomic counter replaces the channel-based dispatch: workers increment a
// shared index to claim the next job, providing dynamic load balancing (important
// for BA graphs where hub ego-nets are much larger) without channel overhead.
// Results are written at the same index as the corresponding job.
func runParallelEgoNets(jobs []egoNetJob, det CommunityDetector, workerCount int) []egoNetResult {
	if len(jobs) == 0 {
		return nil
	}
	out := make([]egoNetResult, len(jobs))
	var idx atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(localDet CommunityDetector) {
			defer wg.Done()
			for {
				i := int(idx.Add(1)) - 1
				if i >= len(jobs) {
					return
				}
				job := jobs[i]
				res, err := localDet.Detect(job.egoNet)
				releaseGraph(job.egoNet) // safe: Detect is done; no other live reference
				out[i] = egoNetResult{v: job.v, partition: res.Partition, err: err}
			}
		}(cloneDetector(det))
	}
	wg.Wait()
	return out
}

// runParallelBuildDetect builds ego-nets AND runs detection in parallel across
// workerCount goroutines. Each worker claims a node index via atomic counter,
// builds the ego-net with per-goroutine scratch buffers, then runs detection.
//
// Per-goroutine graph reuse: each goroutine owns one *Graph cleared in-place
// between ego nets, eliminating pool.Get/Put overhead and pool.New allocs per
// ego-net. edgeBacking and sortedIDsBuf grow lazily within each goroutine.
// partition==nil in a result signals an isolated node (empty ego-net, no error).
func runParallelBuildDetect(nodes []NodeID, g *Graph, det CommunityDetector, workerCount int) []egoNetResult {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]egoNetResult, len(nodes))
	var idx atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func(localDet CommunityDetector) {
			defer wg.Done()
			var scratch []NodeID
			var edgeBacking []Edge
			var sortedIDsBuf []NodeID
			var degreesBuf []int
			for {
				i := int(idx.Add(1)) - 1
				if i >= len(nodes) {
					return
				}
				v := nodes[i]
				scratch = scratch[:0]
				for _, e := range g.Neighbors(v) {
					scratch = append(scratch, e.To)
				}
				egoNet := subgraphWithScratch(g, scratch, &edgeBacking, &sortedIDsBuf, &degreesBuf)
				if egoNet.NodeCount() == 0 {
					releaseGraph(egoNet)
					out[i] = egoNetResult{v: v} // partition==nil signals isolated
					continue
				}
				res, err := localDet.Detect(egoNet)
				releaseGraph(egoNet)
				out[i] = egoNetResult{v: v, partition: res.Partition, err: err}
			}
		}(cloneDetector(det))
	}
	wg.Wait()
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

// buildEgoNetWithScratch is like buildEgoNet but reuses scratch to avoid allocating
// the intermediate nodeIDs slice. The scratch slice is owned by the caller and must
// not be used concurrently. Used by buildPersonaGraph to eliminate one alloc per node.
func buildEgoNetWithScratch(g *Graph, v NodeID, scratch *[]NodeID) *Graph {
	neighbors := g.Neighbors(v)
	*scratch = (*scratch)[:0]
	for _, e := range neighbors {
		*scratch = append(*scratch, e.To)
	}
	return g.Subgraph(*scratch)
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

	n := len(g.Nodes())
	personaOf := make(map[NodeID]map[int]NodeID, n)
	// inverseMap persona count ≈ nodes × avg-communities; len(nodes) is a lower bound
	// that avoids repeated rehashing in the common case (≤1 community per node).
	inverseMap := make(map[NodeID]NodeID, n)
	// partitions[v] holds the ego-net partition for v: neighbor -> community ID in G_v
	partitions := make(map[NodeID]map[NodeID]int, n)

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

	// Build ego-nets AND detect in parallel: each worker claims a node index,
	// builds the ego-net with a per-goroutine scratch slice, runs detection, and
	// releases the graph — eliminating the sequential main-goroutine build phase.
	results := runParallelBuildDetect(nodes, g, localDetector, workerCount)

	// Check for errors first.
	for _, r := range results {
		if r.err != nil {
			return nil, nil, nil, nil, r.err
		}
	}

	// Assign persona nodes for all results (isolated and non-empty alike).
	// commIDsBuf is a reused scratch slice for sort+dedup — avoids per-node map allocation.
	var commIDsBuf []int
	for _, r := range results {
		v := r.v
		personaOf[v] = make(map[int]NodeID, 2) // most nodes have 1–2 communities
		if r.partition == nil {
			// Isolated node: single persona, community 0, no detection needed.
			personaOf[v][0] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
			partitions[v] = make(map[NodeID]int)
			continue
		}
		partitions[v] = r.partition

		commIDsBuf = commIDsBuf[:0]
		for _, commID := range r.partition {
			commIDsBuf = append(commIDsBuf, commID)
		}
		slices.Sort(commIDsBuf)
		prev := -1
		for _, commID := range commIDsBuf {
			if commID == prev {
				continue
			}
			prev = commID
			personaOf[v][commID] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
		}
	}

	// Step 4-6: build persona graph with pre-sized adjacency to avoid append growth allocs.
	//
	// Two-pass approach (mirrors buildSupergraph's single backing-array strategy):
	//   Pass 1: resolve each original edge to (personaU, personaV, weight) and count degrees.
	//   Pass 2: pre-allocate adjacency from one backing array, then wire edges directly.
	type pEdge struct {
		u, v NodeID
		w    float64
	}
	pEdges := make([]pEdge, 0, g.EdgeCount())
	// PersonaIDs are assigned sequentially from basePersona = maxNodeID+1 up to nextPersona-1.
	// Use a []int indexed by (personaID - basePersona) instead of map[NodeID]int to avoid
	// a large map allocation for every buildPersonaGraph call.
	basePersona := maxNodeID + 1
	personaDegree := make([]int, int(nextPersona-basePersona))
	for _, u := range g.Nodes() {
		for _, e := range g.Neighbors(u) {
			v := e.To
			if v < u {
				continue // canonical direction: process each undirected edge once
			}
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
			pEdges = append(pEdges, pEdge{personaU, personaV, e.Weight})
			personaDegree[personaU-basePersona]++
			if personaU != personaV {
				personaDegree[personaV-basePersona]++
			}
		}
	}

	// Pre-allocate adjacency backing: one []Edge slice covers all adjacency lists.
	// For undirected, each edge stores two slots (one per endpoint).
	totalSlots := 0
	for _, d := range personaDegree {
		totalSlots += d
	}
	edgeBacking := make([]Edge, totalSlots)

	// Persona graph is long-lived (returned to caller) — allocate directly pre-sized
	// rather than acquiring from graphPool, to avoid bucket-growth on a fresh pool entry
	// and to avoid inadvertently releasing a long-lived graph back to the pool.
	numPersonas := int(nextPersona - basePersona)
	personaGraph := &Graph{
		directed:  false,
		nodes:     make(map[NodeID]float64, numPersonas),
		adjacency: make(map[NodeID][]Edge, numPersonas),
	}
	// PersonaIDs are assigned sequentially from basePersona to nextPersona-1.
	// Iterate sequentially to avoid maps.(*Iter).Next overhead and pre-set sortedNodes
	// (eliminating the Nodes() map-iteration + sort on the first global-detector call).
	sortedPersonas := make([]NodeID, numPersonas)
	off := 0
	for idx := range numPersonas {
		personaID := NodeID(int(basePersona) + idx)
		sortedPersonas[idx] = personaID
		personaGraph.nodes[personaID] = 1.0
		d := personaDegree[idx]
		if d > 0 {
			personaGraph.adjacency[personaID] = edgeBacking[off : off : off+d]
			off += d
		}
	}
	personaGraph.sortedNodes = sortedPersonas

	// Wire edges directly into pre-allocated adjacency slices.
	for _, pe := range pEdges {
		personaGraph.adjacency[pe.u] = append(personaGraph.adjacency[pe.u], Edge{To: pe.v, Weight: pe.w})
		personaGraph.totalWeight += pe.w
		if pe.u != pe.v {
			personaGraph.adjacency[pe.v] = append(personaGraph.adjacency[pe.v], Edge{To: pe.u, Weight: pe.w})
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
	var nonEmptyAffected []egoNetJob
	for v := range affected {
		egoNet := buildEgoNet(g, v)
		if egoNet.NodeCount() == 0 {
			personaOf[v][0] = nextPersona
			inverseMap[nextPersona] = v
			nextPersona++
			partitions[v] = make(map[NodeID]int)
			continue
		}
		nonEmptyAffected = append(nonEmptyAffected, egoNetJob{v: v, egoNet: egoNet})
	}

	if len(nonEmptyAffected) > 0 {
		results := runParallelEgoNets(nonEmptyAffected, localDetector, workerCount)

		for _, r := range results {
			if r.err != nil {
				return nil, nil, nil, nil, nil, false, r.err
			}
		}

		var commIDsBuf2 []int
		for _, r := range results {
			v := r.v
			partitions[v] = r.partition

			commIDsBuf2 = commIDsBuf2[:0]
			for _, commID := range r.partition {
				commIDsBuf2 = append(commIDsBuf2, commID)
			}
			slices.Sort(commIDsBuf2)
			prev := -1
			for _, commID := range commIDsBuf2 {
				if commID == prev {
					continue
				}
				prev = commID
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
		// Dedup: for edges where both endpoints are in affected, only process from the
		// smaller-ID side (u < v), avoiding the seen-map allocation.
		for u := range affected {
			for _, e := range g.Neighbors(u) {
				v := e.To
				// Skip if both endpoints are affected and v < u (already processed from v's side).
				if _, vAffected := affected[v]; vAffected && v < u {
					continue
				}

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

		for _, u := range g.Nodes() {
			for _, e := range g.Neighbors(u) {
				v := e.To
				if v < u {
					continue // process each undirected edge once (u ≤ v canonical)
				}

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

	// Deduplicate community IDs per node using sort+compact (zero allocations per node).
	// mapPersonasToOriginal can emit duplicate IDs when multiple personas of the
	// same original node land in the same global community.
	for node, comms := range nodeCommunities {
		if len(comms) <= 1 {
			continue
		}
		slices.Sort(comms)
		out := comms[:1]
		for _, c := range comms[1:] {
			if c != out[len(out)-1] {
				out = append(out, c)
			}
		}
		nodeCommunities[node] = out
	}

	// Build Communities [][]NodeID using counting-sort strategy:
	//   1. Count members per community with a flat []int (8x smaller than [][]NodeID).
	//   2. Pre-allocate one contiguous NodeID backing array for all community slices.
	//   3. Populate + remap nodeCommunities in one combined pass.
	//
	// This replaces the previous approach which allocated a [][]NodeID of size
	// maxComm+1 (up to ~80K elements for sparse persona graphs) and grew `filtered`
	// via repeated append, causing large intermediate allocations.
	maxComm := -1
	for _, comms := range nodeCommunities {
		for _, c := range comms {
			if c > maxComm {
				maxComm = c
			}
		}
	}
	if maxComm < 0 {
		return nil, nodeCommunities
	}

	// Pass 1: count members per community.
	counts := make([]int, maxComm+1)
	for _, comms := range nodeCommunities {
		for _, c := range comms {
			counts[c]++
		}
	}

	// Compute how many non-empty communities exist and total member slots needed.
	numComms := 0
	totalMembers := 0
	for _, cnt := range counts {
		if cnt > 0 {
			numComms++
			totalMembers += cnt
		}
	}

	// commRemap[c] = index in filtered for community c (-1 = empty).
	commRemap := make([]int, maxComm+1)
	for i := range commRemap {
		commRemap[i] = -1
	}

	// Single backing array covers all community member slices — one allocation total.
	memberBacking := make([]NodeID, totalMembers)
	filtered := make([][]NodeID, 0, numComms)
	off := 0
	for i, cnt := range counts {
		if cnt > 0 {
			commRemap[i] = len(filtered)
			filtered = append(filtered, memberBacking[off:off:off+cnt])
			off += cnt
		}
	}

	// Pass 2: populate filtered + remap nodeCommunities IDs in-place (combined).
	for node, comms := range nodeCommunities {
		for j, c := range comms {
			idx := commRemap[c]
			filtered[idx] = append(filtered[idx], node)
			comms[j] = idx
		}
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
	// Pass 1: count personas per original node.
	counts := make(map[NodeID]int, len(inverseMap))
	for personaID := range globalPartition {
		if origNode, ok := inverseMap[personaID]; ok {
			counts[origNode]++
		}
	}

	// Single backing array for all result slices — eliminates N per-node make calls.
	// Each original node's community slice is a view into this array.
	total := 0
	for _, cnt := range counts {
		total += cnt
	}
	backing := make([]int, total)
	result := make(map[NodeID][]int, len(counts))
	off := 0
	for origNode, cnt := range counts {
		result[origNode] = backing[off : off : off+cnt]
		off += cnt
	}

	// Pass 2: fill community IDs into pre-sized slices.
	for personaID, commID := range globalPartition {
		origNode, ok := inverseMap[personaID]
		if !ok {
			continue
		}
		result[origNode] = append(result[origNode], commID)
	}
	return result
}
