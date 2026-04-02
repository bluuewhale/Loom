package graph

import (
	"slices"
	"sync"
)

// NodeID is a type-safe identifier for graph nodes.
type NodeID int

// Edge represents a weighted directed edge to a neighbor node.
type Edge struct {
	To     NodeID
	Weight float64
}

// Graph is a weighted directed or undirected graph backed by an adjacency list.
// For undirected graphs, AddEdge stores both directions but counts each distinct
// edge weight only once in totalWeight.
type Graph struct {
	directed    bool
	nodes       map[NodeID]float64 // nodeID -> node weight (default 1.0)
	adjacency   map[NodeID][]Edge  // nodeID -> neighbor edges
	totalWeight float64            // sum of distinct edge weights
	sortedNodes []NodeID           // cache; nil when stale
}

// NewGraph creates a new empty graph. If directed is true, edges are one-way;
// otherwise both directions are stored for each edge.
func NewGraph(directed bool) *Graph {
	return &Graph{
		directed:  directed,
		nodes:     make(map[NodeID]float64),
		adjacency: make(map[NodeID][]Edge),
	}
}

// newGraphSized returns a Graph ready to hold n nodes. It acquires from graphPool
// so callers that call releaseGraph return it for reuse; callers that do not are
// safe — the object is GC-collected normally. The n hint is retained for
// documentation; pool reuse means actual map capacity may be larger.
func newGraphSized(directed bool, n int) *Graph {
	return acquireGraph(directed)
}

// graphPool pools *Graph objects to reduce allocation pressure from repeated
// ego-net and supergraph creation. Callers that want to return a Graph to the
// pool must call releaseGraph — callers that do not are safe; the object is
// simply GC-collected normally.
var graphPool = sync.Pool{
	New: func() any {
		return &Graph{
			nodes:     make(map[NodeID]float64, 64),
			adjacency: make(map[NodeID][]Edge, 64),
		}
	},
}

// acquireGraph obtains a Graph from the pool. The maps are cleared and metadata
// fields are reset; retained bucket capacity avoids rehashing on reuse.
func acquireGraph(directed bool) *Graph {
	g := graphPool.Get().(*Graph)
	clear(g.nodes)
	clear(g.adjacency)
	g.directed = directed
	g.totalWeight = 0
	g.sortedNodes = nil
	return g
}

// releaseGraph returns g to the pool. Callers must not use g after this call,
// and must ensure no other live reference to g exists.
func releaseGraph(g *Graph) {
	graphPool.Put(g)
}

// subgraphNodeSetPool reuses the nodeSet membership map for Subgraph calls.
var subgraphNodeSetPool = sync.Pool{
	New: func() any {
		m := make(map[NodeID]struct{}, 32)
		return &m
	},
}

// subgraphDegreesPool reuses the per-node degree scratch slice in Subgraph.
// Eliminates one allocation per Subgraph call (10K+ calls during EgoSplitting).
var subgraphDegreesPool = sync.Pool{
	New: func() any {
		s := make([]int, 0, 32)
		return &s
	},
}

// AddNode adds a node with the given weight. If the node already exists, this is a no-op.
func (g *Graph) AddNode(id NodeID, weight float64) {
	if _, exists := g.nodes[id]; !exists {
		g.nodes[id] = weight
		// Ensure adjacency entry exists even if no edges added
		if _, ok := g.adjacency[id]; !ok {
			g.adjacency[id] = nil
		}
		g.sortedNodes = nil
	}
}

// AddEdge adds a weighted edge from 'from' to 'to'. If the nodes do not exist,
// they are auto-created with weight 1.0. For undirected graphs, the reverse edge
// is also added. Self-loops (from == to) store a single adjacency entry.
// totalWeight is incremented once per AddEdge call regardless of directionality.
func (g *Graph) AddEdge(from, to NodeID, weight float64) {
	// Auto-create nodes if not present
	if _, exists := g.nodes[from]; !exists {
		g.nodes[from] = 1.0
		if _, ok := g.adjacency[from]; !ok {
			g.adjacency[from] = nil
		}
	}
	if _, exists := g.nodes[to]; !exists {
		g.nodes[to] = 1.0
		if _, ok := g.adjacency[to]; !ok {
			g.adjacency[to] = nil
		}
	}

	g.adjacency[from] = append(g.adjacency[from], Edge{To: to, Weight: weight})

	if !g.directed && from != to {
		// Undirected: add reverse edge (self-loops only stored once)
		g.adjacency[to] = append(g.adjacency[to], Edge{To: from, Weight: weight})
	}

	// totalWeight counts each distinct edge once
	g.totalWeight += weight
	g.sortedNodes = nil
}

// Neighbors returns the list of edges from node id. Returns nil if node has no edges.
func (g *Graph) Neighbors(id NodeID) []Edge {
	return g.adjacency[id]
}

// Nodes returns all node IDs in sorted order. The returned slice is cached
// internally and MUST NOT be modified by callers. The cache is invalidated
// automatically on AddNode/AddEdge.
func (g *Graph) Nodes() []NodeID {
	if g.sortedNodes != nil {
		return g.sortedNodes
	}
	ids := make([]NodeID, 0, len(g.nodes))
	for id := range g.nodes {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	g.sortedNodes = ids
	return ids
}

// NodeCount returns the number of nodes in the graph.
func (g *Graph) NodeCount() int {
	return len(g.nodes)
}

// EdgeCount returns the number of distinct edges. For undirected graphs, each
// edge is counted once (adjacency entries / 2).
func (g *Graph) EdgeCount() int {
	total := 0
	for _, edges := range g.adjacency {
		total += len(edges)
	}
	if !g.directed {
		return total / 2
	}
	return total
}

// TotalWeight returns the sum of all distinct edge weights.
func (g *Graph) TotalWeight() float64 {
	return g.totalWeight
}

// Strength returns the sum of weights of all edges incident to node n.
// For undirected graphs, self-loops contribute their weight once (stored once
// in adjacency); regular edges are stored in both directions so each contributes once here.
func (g *Graph) Strength(n NodeID) float64 {
	var s float64
	for _, e := range g.adjacency[n] {
		s += e.Weight
	}
	return s
}

// RemoveEdgesFor removes all edges incident to any node in the nodeSet from the
// graph. For undirected graphs, both directions are removed. totalWeight is
// decremented for each removed edge. Nodes themselves are not removed.
// This is used by buildPersonaGraphIncremental to surgically update the cloned
// prior persona graph when only a small set of personas change.
func (g *Graph) RemoveEdgesFor(nodeSet map[NodeID]struct{}) {
	// Collect the weight that will be removed to keep totalWeight accurate.
	// For undirected: each edge (u,v) is stored in both adjacency[u] and
	// adjacency[v]. We credit the weight once when we remove from u's list
	// (the canonical source), and then just filter v's list silently.
	removedWeight := 0.0

	for u := range nodeSet {
		for _, e := range g.adjacency[u] {
			// For undirected, credit weight once per distinct edge removed from u.
			removedWeight += e.Weight
			// Remove the reverse edge at e.To if not also in nodeSet
			// (if both endpoints are in nodeSet, we'll clear e.To's list anyway).
			if _, toAlso := nodeSet[e.To]; !toAlso && !g.directed && u != e.To {
				filtered := g.adjacency[e.To][:0]
				for _, re := range g.adjacency[e.To] {
					if re.To != u {
						filtered = append(filtered, re)
					}
				}
				g.adjacency[e.To] = filtered
			}
		}
		g.adjacency[u] = g.adjacency[u][:0]
	}

	g.totalWeight -= removedWeight
}

// IsDirected returns true if the graph is directed.
func (g *Graph) IsDirected() bool {
	return g.directed
}

// Clone returns a deep copy of the graph. The clone is fully independent of the original.
// Uses a single backing array for all edge slices (same strategy as Subgraph/buildSupergraph),
// reducing alloc count from O(N) per-node slices to O(1) regardless of node count.
func (g *Graph) Clone() *Graph {
	c := &Graph{
		directed:    g.directed,
		totalWeight: g.totalWeight,
		nodes:       make(map[NodeID]float64, len(g.nodes)),
		adjacency:   make(map[NodeID][]Edge, len(g.adjacency)),
	}
	for id, w := range g.nodes {
		c.nodes[id] = w
	}
	// Count total edges for a single backing allocation.
	totalEdges := 0
	for _, edges := range g.adjacency {
		totalEdges += len(edges)
	}
	backing := make([]Edge, totalEdges)
	off := 0
	for id, edges := range g.adjacency {
		if edges == nil {
			c.adjacency[id] = nil
		} else {
			n := len(edges)
			c.adjacency[id] = backing[off : off+n : off+n]
			copy(c.adjacency[id], edges)
			off += n
		}
	}
	return c
}

// Subgraph returns a new graph containing only the specified nodes and the
// edges between them. totalWeight is recalculated from included edges.
func (g *Graph) Subgraph(nodeIDs []NodeID) *Graph {
	// Sort nodeIDs in-place: enables pre-caching sortedNodes (replaces map iteration
	// + sort in Nodes() on first call) and has no effect on the resulting subgraph.
	slices.Sort(nodeIDs)

	nodeSetPtr := subgraphNodeSetPool.Get().(*map[NodeID]struct{})
	nodeSet := *nodeSetPtr
	for k := range nodeSet {
		delete(nodeSet, k)
	}
	defer subgraphNodeSetPool.Put(nodeSetPtr)
	for _, id := range nodeIDs {
		nodeSet[id] = struct{}{}
	}

	// Pre-count degree per node (pass 1): degree[i] = edges from nodeIDs[i] to nodeSet.
	// For undirected graphs, g.adjacency stores both directions, so degree[i] is the
	// exact number of Edge entries that will end up in sub.adjacency[nodeIDs[i]].
	//
	// Pass 2 uses a single contiguous backing array for all adjacency slices,
	// eliminating per-node make([]Edge,...) calls. degrees is pooled to eliminate
	// the one-per-call allocation.
	degreesPtr := subgraphDegreesPool.Get().(*[]int)
	if cap(*degreesPtr) >= len(nodeIDs) {
		*degreesPtr = (*degreesPtr)[:len(nodeIDs)]
		clear(*degreesPtr)
	} else {
		*degreesPtr = make([]int, len(nodeIDs))
	}
	degrees := *degreesPtr
	defer subgraphDegreesPool.Put(degreesPtr)
	totalEdgeSlots := 0
	for i, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := nodeSet[e.To]; inSet {
				degrees[i]++
			}
		}
		totalEdgeSlots += degrees[i]
	}

	// Single backing array covers all adjacency lists — one allocation replaces N.
	// sortedIDs accumulates valid IDs in sorted order to pre-cache sortedNodes,
	// eliminating the Nodes() map-iteration + sort on first call.
	edgeBacking := make([]Edge, totalEdgeSlots)
	sub := newGraphSized(g.directed, len(nodeIDs))
	sortedIDs := make([]NodeID, 0, len(nodeIDs))
	off := 0
	for i, id := range nodeIDs {
		if w, ok := g.nodes[id]; ok {
			sub.nodes[id] = w
			sortedIDs = append(sortedIDs, id) // nodeIDs is sorted → sortedIDs is sorted
			d := degrees[i]
			if d > 0 {
				sub.adjacency[id] = edgeBacking[off : off : off+d]
				off += d
			}
		}
	}
	sub.sortedNodes = sortedIDs

	// Wire edges (pass 2): no growth allocs — adjacency slices are exactly pre-sized.
	// For undirected, process canonical (lo, hi) direction only to count totalWeight once;
	// both directions are written directly into pre-allocated adjacency slices.
	for _, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := nodeSet[e.To]; !inSet {
				continue
			}
			if !g.directed {
				if e.To < from {
					continue // canonical direction: skip reverse
				}
			}
			sub.adjacency[from] = append(sub.adjacency[from], Edge{To: e.To, Weight: e.Weight})
			sub.totalWeight += e.Weight
			if !g.directed && from != e.To {
				sub.adjacency[e.To] = append(sub.adjacency[e.To], Edge{To: from, Weight: e.Weight})
			}
		}
	}
	return sub
}

// subgraphWithScratch is like Subgraph but reuses caller-provided edgeBacking and
// sortedIDsBuf across calls to eliminate 2 allocs per ego-net build.
// The graph itself is still acquired from graphPool (avoids O(buckets) clear cost
// on large maps that accumulate after hub-node processing).
// nodeIDs is sorted in-place; edgeBacking, sortedIDsBuf, and degreesBuf grow lazily.
// Membership test uses binary search on the sorted nodeIDs slice instead of a shared
// pool-based hash map, eliminating pool contention and hash overhead for small sets.
func subgraphWithScratch(g *Graph, nodeIDs []NodeID, edgeBacking *[]Edge, sortedIDsBuf *[]NodeID, degreesBuf *[]int) *Graph {
	slices.Sort(nodeIDs)

	// Grow degreesBuf lazily (per-goroutine scratch — no pool contention).
	if cap(*degreesBuf) < len(nodeIDs) {
		*degreesBuf = make([]int, len(nodeIDs), len(nodeIDs)+len(nodeIDs)/4+1)
	} else {
		*degreesBuf = (*degreesBuf)[:len(nodeIDs)]
		clear(*degreesBuf)
	}
	degrees := *degreesBuf

	totalEdgeSlots := 0
	for i, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := slices.BinarySearch(nodeIDs, e.To); inSet {
				degrees[i]++
			}
		}
		totalEdgeSlots += degrees[i]
	}

	// Grow edgeBacking lazily: eliminates per-call make([]Edge, N) alloc.
	if cap(*edgeBacking) < totalEdgeSlots {
		*edgeBacking = make([]Edge, totalEdgeSlots, totalEdgeSlots+totalEdgeSlots/4+1)
	} else {
		*edgeBacking = (*edgeBacking)[:totalEdgeSlots]
	}
	eb := *edgeBacking

	// Grow sortedIDsBuf lazily; accumulates valid IDs in sorted order.
	if cap(*sortedIDsBuf) < len(nodeIDs) {
		*sortedIDsBuf = make([]NodeID, 0, len(nodeIDs)+len(nodeIDs)/4+1)
	} else {
		*sortedIDsBuf = (*sortedIDsBuf)[:0]
	}

	sub := newGraphSized(g.directed, len(nodeIDs))
	off := 0
	for i, id := range nodeIDs {
		if w, ok := g.nodes[id]; ok {
			sub.nodes[id] = w
			*sortedIDsBuf = append(*sortedIDsBuf, id)
			d := degrees[i]
			if d > 0 {
				sub.adjacency[id] = eb[off : off : off+d]
				off += d
			}
		}
	}
	sub.sortedNodes = *sortedIDsBuf

	for _, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := slices.BinarySearch(nodeIDs, e.To); !inSet {
				continue
			}
			if !g.directed {
				if e.To < from {
					continue
				}
			}
			sub.adjacency[from] = append(sub.adjacency[from], Edge{To: e.To, Weight: e.Weight})
			sub.totalWeight += e.Weight
			if !g.directed && from != e.To {
				sub.adjacency[e.To] = append(sub.adjacency[e.To], Edge{To: from, Weight: e.Weight})
			}
		}
	}
	return sub
}

// subgraphInto rebuilds dst in-place as the subgraph of g induced by nodeIDs,
// reusing caller-provided scratch buffers to eliminate all per-call allocations.
// nodeIDs is sorted in-place (order has no effect on the resulting subgraph).
// edgeBacking and sortedIDsBuf grow lazily and must not be shared across goroutines.
// dst must be owned by the caller; it is cleared and rebuilt on each call.
func subgraphInto(g *Graph, nodeIDs []NodeID, edgeBacking *[]Edge, sortedIDsBuf *[]NodeID, dst *Graph) {
	// Clear dst in-place: retain map bucket capacity (avoids rehashing on reuse).
	clear(dst.nodes)
	clear(dst.adjacency)
	dst.directed = g.directed
	dst.totalWeight = 0
	dst.sortedNodes = nil

	if len(nodeIDs) == 0 {
		return
	}

	slices.Sort(nodeIDs)

	nodeSetPtr := subgraphNodeSetPool.Get().(*map[NodeID]struct{})
	nodeSet := *nodeSetPtr
	for k := range nodeSet {
		delete(nodeSet, k)
	}
	defer subgraphNodeSetPool.Put(nodeSetPtr)
	for _, id := range nodeIDs {
		nodeSet[id] = struct{}{}
	}

	degreesPtr := subgraphDegreesPool.Get().(*[]int)
	if cap(*degreesPtr) >= len(nodeIDs) {
		*degreesPtr = (*degreesPtr)[:len(nodeIDs)]
		clear(*degreesPtr)
	} else {
		*degreesPtr = make([]int, len(nodeIDs))
	}
	degrees := *degreesPtr
	defer subgraphDegreesPool.Put(degreesPtr)
	totalEdgeSlots := 0
	for i, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := nodeSet[e.To]; inSet {
				degrees[i]++
			}
		}
		totalEdgeSlots += degrees[i]
	}

	// Grow edgeBacking lazily; reuse across calls eliminates per-call alloc.
	if cap(*edgeBacking) < totalEdgeSlots {
		*edgeBacking = make([]Edge, totalEdgeSlots, totalEdgeSlots+totalEdgeSlots/4+1)
	} else {
		*edgeBacking = (*edgeBacking)[:totalEdgeSlots]
	}
	eb := *edgeBacking

	// Grow sortedIDsBuf lazily; accumulates valid IDs in sorted order.
	if cap(*sortedIDsBuf) < len(nodeIDs) {
		*sortedIDsBuf = make([]NodeID, 0, len(nodeIDs)+len(nodeIDs)/4+1)
	} else {
		*sortedIDsBuf = (*sortedIDsBuf)[:0]
	}

	off := 0
	for i, id := range nodeIDs {
		if w, ok := g.nodes[id]; ok {
			dst.nodes[id] = w
			*sortedIDsBuf = append(*sortedIDsBuf, id)
			d := degrees[i]
			if d > 0 {
				dst.adjacency[id] = eb[off : off : off+d]
				off += d
			}
		}
	}
	dst.sortedNodes = *sortedIDsBuf

	for _, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := nodeSet[e.To]; !inSet {
				continue
			}
			if !g.directed {
				if e.To < from {
					continue
				}
			}
			dst.adjacency[from] = append(dst.adjacency[from], Edge{To: e.To, Weight: e.Weight})
			dst.totalWeight += e.Weight
			if !g.directed && from != e.To {
				dst.adjacency[e.To] = append(dst.adjacency[e.To], Edge{To: from, Weight: e.Weight})
			}
		}
	}
}

// WeightToComm returns the sum of edge weights from node n to all nodes in community comm,
// according to the provided partition map.
func (g *Graph) WeightToComm(n NodeID, comm int, partition map[NodeID]int) float64 {
	var w float64
	for _, e := range g.adjacency[n] {
		if partition[e.To] == comm {
			w += e.Weight
		}
	}
	return w
}

// CommStrength returns the sum of Strength(n) for all nodes n in community comm.
// O(n): iterates the full partition. Do not call inside inner loops — use a
// precomputed commStr cache instead.
func (g *Graph) CommStrength(comm int, partition map[NodeID]int) float64 {
	var total float64
	for n, c := range partition {
		if c == comm {
			total += g.Strength(n)
		}
	}
	return total
}
