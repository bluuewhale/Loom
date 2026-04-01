package graph

import "slices"

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
	for id, edges := range g.adjacency {
		if edges == nil {
			c.adjacency[id] = nil
		} else {
			copied := make([]Edge, len(edges))
			copy(copied, edges)
			c.adjacency[id] = copied
		}
	}
	return c
}

// Subgraph returns a new graph containing only the specified nodes and the
// edges between them. totalWeight is recalculated from included edges.
func (g *Graph) Subgraph(nodeIDs []NodeID) *Graph {
	nodeSet := make(map[NodeID]struct{}, len(nodeIDs))
	for _, id := range nodeIDs {
		nodeSet[id] = struct{}{}
	}

	sub := NewGraph(g.directed)
	// Add nodes
	for _, id := range nodeIDs {
		if w, ok := g.nodes[id]; ok {
			sub.nodes[id] = w
			if _, ok2 := sub.adjacency[id]; !ok2 {
				sub.adjacency[id] = nil
			}
		}
	}

	// Add edges where both endpoints are in nodeSet.
	// For undirected graphs, we process each edge only once to avoid double-counting totalWeight.
	seen := make(map[[2]NodeID]struct{})
	for _, from := range nodeIDs {
		for _, e := range g.adjacency[from] {
			if _, inSet := nodeSet[e.To]; !inSet {
				continue
			}
			if !g.directed {
				// Use canonical key (min, max) to avoid processing both directions
				lo, hi := from, e.To
				if lo > hi {
					lo, hi = hi, lo
				}
				key := [2]NodeID{lo, hi}
				if _, already := seen[key]; already {
					continue
				}
				seen[key] = struct{}{}
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
