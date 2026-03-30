package graph

import (
	"errors"
	"math"
	"sort"
	"testing"

	"github.com/bluuewhale/loom/graph/testdata"
)

// Compile-time interface satisfaction check.
var _ OverlappingCommunityDetector = (*egoSplittingDetector)(nil)

// Test 1: NewEgoSplitting returns a non-nil value satisfying OverlappingCommunityDetector.
func TestNewEgoSplitting_ReturnsOverlappingCommunityDetector(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	if d == nil {
		t.Fatal("NewEgoSplitting returned nil")
	}
}

// Test 2: NewEgoSplitting defaults nil LocalDetector and GlobalDetector to Louvain.
func TestNewEgoSplitting_DefaultsNilDetectors(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	impl, ok := d.(*egoSplittingDetector)
	if !ok {
		t.Fatal("NewEgoSplitting did not return *egoSplittingDetector")
	}
	if impl.opts.LocalDetector == nil {
		t.Error("LocalDetector is nil after NewEgoSplitting with zero options")
	}
	if impl.opts.GlobalDetector == nil {
		t.Error("GlobalDetector is nil after NewEgoSplitting with zero options")
	}
	if impl.opts.Resolution != 1.0 {
		t.Errorf("Resolution = %v, want 1.0", impl.opts.Resolution)
	}
}

// Test 3: NewEgoSplitting preserves caller-supplied detectors.
func TestNewEgoSplitting_PreservesSuppliedDetectors(t *testing.T) {
	local := NewLeiden(LeidenOptions{})
	global := NewLeiden(LeidenOptions{})
	d := NewEgoSplitting(EgoSplittingOptions{
		LocalDetector:  local,
		GlobalDetector: global,
		Resolution:     2.0,
	})
	impl := d.(*egoSplittingDetector)
	if impl.opts.LocalDetector != local {
		t.Error("LocalDetector was overwritten")
	}
	if impl.opts.GlobalDetector != global {
		t.Error("GlobalDetector was overwritten")
	}
	if impl.opts.Resolution != 2.0 {
		t.Errorf("Resolution = %v, want 2.0", impl.opts.Resolution)
	}
}

// Test 4: Detect stub returns ErrNotImplemented.
func TestEgoSplittingDetector_Detect_ReturnsErrNotImplemented(t *testing.T) {
	d := NewEgoSplitting(EgoSplittingOptions{})
	g := NewGraph(false)
	_, err := d.Detect(g)
	if err == nil {
		t.Fatal("expected ErrNotImplemented, got nil")
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got: %v", err)
	}
}

// Test 5: OverlappingCommunityResult zero-value has expected field defaults.
func TestOverlappingCommunityResult_ZeroValue(t *testing.T) {
	r := OverlappingCommunityResult{}
	if r.Communities != nil {
		t.Errorf("expected Communities nil, got %v", r.Communities)
	}
	if r.NodeCommunities != nil {
		t.Errorf("expected NodeCommunities nil, got %v", r.NodeCommunities)
	}
}

// --- helpers ---

func makeTriangle() *Graph {
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(0, 2, 1.0)
	g.AddEdge(1, 2, 1.0)
	return g
}

func makeBarbell() *Graph {
	// 4-node barbell: 0-1, 0-2, 1-2 (triangle) + 2-3 (bridge)
	g := NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(0, 2, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 3, 1.0)
	return g
}

// --- buildEgoNet tests ---

// TestBuildEgoNet_Triangle: ego-net of node 0 in triangle contains only {1,2} and edge (1,2).
func TestBuildEgoNet_Triangle(t *testing.T) {
	g := makeTriangle()
	ego := buildEgoNet(g, 0)

	nodes := ego.Nodes()
	if len(nodes) != 2 {
		t.Fatalf("ego-net of 0 has %d nodes, want 2", len(nodes))
	}

	nodeSet := make(map[NodeID]struct{})
	for _, n := range nodes {
		nodeSet[n] = struct{}{}
	}
	if _, ok := nodeSet[0]; ok {
		t.Error("ego node 0 should not be in its own ego-net")
	}
	if _, ok := nodeSet[1]; !ok {
		t.Error("node 1 should be in ego-net of 0")
	}
	if _, ok := nodeSet[2]; !ok {
		t.Error("node 2 should be in ego-net of 0")
	}

	// Edge (1,2) must exist
	if ego.EdgeCount() != 1 {
		t.Errorf("ego-net of 0 should have 1 edge (1-2), got %d", ego.EdgeCount())
	}
}

// TestBuildEgoNet_ExcludesEgoNode: for every node v, v is not in buildEgoNet(g, v).Nodes().
func TestBuildEgoNet_ExcludesEgoNode(t *testing.T) {
	g := makeBarbell()
	for _, v := range g.Nodes() {
		ego := buildEgoNet(g, v)
		for _, n := range ego.Nodes() {
			if n == v {
				t.Errorf("ego node %d appears in its own ego-net", v)
			}
		}
	}
}

// --- buildPersonaGraph tests ---

// TestBuildPersonaGraph_Triangle: persona graph has 3+ nodes, all PersonaIDs >= 3,
// and TotalWeight equals original graph's TotalWeight.
func TestBuildPersonaGraph_Triangle(t *testing.T) {
	g := makeTriangle()
	localDetector := NewLouvain(LouvainOptions{})

	personaGraph, personaOf, inverseMap, err := buildPersonaGraph(g, localDetector)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	// All PersonaIDs must be >= 3 (maxNodeID=2, so nextPersona starts at 3)
	for personaID := range inverseMap {
		if personaID < 3 {
			t.Errorf("PersonaID %d is < 3 (maxNodeID+1), violates EGO-CRIT-02", personaID)
		}
	}

	// personaOf must have entries for all original nodes
	for _, v := range g.Nodes() {
		if _, ok := personaOf[v]; !ok {
			t.Errorf("node %d missing from personaOf", v)
		}
	}

	// TotalWeight must be preserved
	if personaGraph.TotalWeight() != g.TotalWeight() {
		t.Errorf("personaGraph.TotalWeight() = %v, want %v", personaGraph.TotalWeight(), g.TotalWeight())
	}
}

// TestBuildPersonaGraph_Barbell: 4-node barbell graph; PersonaID space disjoint from [0,4),
// and TotalWeight preserved.
func TestBuildPersonaGraph_Barbell(t *testing.T) {
	g := makeBarbell()
	localDetector := NewLouvain(LouvainOptions{})

	personaGraph, _, inverseMap, err := buildPersonaGraph(g, localDetector)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	// All PersonaIDs must be >= g.NodeCount() (i.e., >= 4 since nodes are 0,1,2,3)
	// More precisely, >= maxNodeID+1 = 4
	for personaID := range inverseMap {
		if int(personaID) < g.NodeCount() {
			t.Errorf("PersonaID %d collides with original node ID space [0,%d)", personaID, g.NodeCount())
		}
	}

	// TotalWeight must be preserved
	if personaGraph.TotalWeight() != g.TotalWeight() {
		t.Errorf("personaGraph.TotalWeight() = %v, want %v", personaGraph.TotalWeight(), g.TotalWeight())
	}
}

// TestBuildPersonaGraph_PersonaIDsDisjoint: no PersonaID falls in [0, g.NodeCount()).
func TestBuildPersonaGraph_PersonaIDsDisjoint(t *testing.T) {
	g := makeTriangle()
	localDetector := NewLouvain(LouvainOptions{})

	_, _, inverseMap, err := buildPersonaGraph(g, localDetector)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	n := g.NodeCount()
	for personaID := range inverseMap {
		if int(personaID) < n {
			t.Errorf("PersonaID %d is in original node range [0,%d)", personaID, n)
		}
	}
}

// TestMapPersonasToOriginal_Bijective: every PersonaID in persona graph maps to exactly
// one original NodeID via inverseMap, and every PersonaID is accounted for.
func TestMapPersonasToOriginal_Bijective(t *testing.T) {
	g := makeTriangle()
	localDetector := NewLouvain(LouvainOptions{})

	personaGraph, _, inverseMap, err := buildPersonaGraph(g, localDetector)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	// Build a fake global partition: assign each persona to community 0
	globalPartition := make(map[NodeID]int)
	for _, personaID := range personaGraph.Nodes() {
		globalPartition[personaID] = 0
	}

	result := mapPersonasToOriginal(globalPartition, inverseMap)

	// Every PersonaID in inverseMap must map to a valid original node
	for personaID, origNode := range inverseMap {
		found := false
		for _, id := range g.Nodes() {
			if id == origNode {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("inverseMap[%d] = %d, but %d is not in original graph", personaID, origNode, origNode)
		}
	}

	// result must contain all original nodes
	originalNodes := g.Nodes()
	sort.Slice(originalNodes, func(i, j int) bool { return originalNodes[i] < originalNodes[j] })
	for _, n := range originalNodes {
		if _, ok := result[n]; !ok {
			t.Errorf("original node %d missing from mapPersonasToOriginal result", n)
		}
	}
}

// --- Karate Club integration tests (Algorithm 1+2+3 end-to-end) ---

// TestPersonaGraphKarateClub_OverlappingMembership validates the full pipeline on
// Zachary's Karate Club graph: buildPersonaGraph -> GlobalDetector.Detect ->
// mapPersonasToOriginal. Asserts weight conservation, collision-free PersonaID
// space, and at least one node with overlapping community membership.
func TestPersonaGraphKarateClub_OverlappingMembership(t *testing.T) {
	// Build Karate Club graph (34 nodes, 78 edges, nodes 0-33).
	g := buildGraph(testdata.KarateClubEdges)

	// Step 1: build persona graph with seeded Louvain as local detector.
	local := NewLouvain(LouvainOptions{Seed: 42})
	personaGraph, _, inverseMap, err := buildPersonaGraph(g, local)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	// (a) Weight conservation: personaGraph.TotalWeight() == g.TotalWeight() within 1e-9.
	if math.Abs(personaGraph.TotalWeight()-g.TotalWeight()) > 1e-9 {
		t.Errorf("TotalWeight mismatch: personaGraph=%v, original=%v", personaGraph.TotalWeight(), g.TotalWeight())
	}

	// (b) All PersonaIDs must be >= 34 (original node range is 0-33).
	for _, id := range personaGraph.Nodes() {
		if int(id) < 34 {
			t.Errorf("PersonaID %d collides with original node space [0,34)", id)
		}
	}

	// Step 2: run global Louvain on persona graph.
	global := NewLouvain(LouvainOptions{Seed: 42})
	globalResult, err := global.Detect(personaGraph)
	if err != nil {
		t.Fatalf("global Detect error: %v", err)
	}

	// Step 3: map personas back to original nodes.
	nodeCommunities := mapPersonasToOriginal(globalResult.Partition, inverseMap)

	// (c) At least one original node has overlapping membership (len > 1).
	hasOverlap := false
	for _, comms := range nodeCommunities {
		if len(comms) > 1 {
			hasOverlap = true
			break
		}
	}
	if !hasOverlap {
		t.Error("expected at least one node with overlapping membership (len(communities) > 1), got none")
	}

	// (d) All 34 original nodes (0-33) must appear in community assignments.
	for i := 0; i < 34; i++ {
		if _, ok := nodeCommunities[NodeID(i)]; !ok {
			t.Errorf("original node %d missing from community assignments", i)
		}
	}

	t.Logf("KarateClub persona graph: %d persona nodes, %d original communities detected",
		personaGraph.NodeCount(), len(nodeCommunities))
}

// TestPersonaGraphKarateClub_AllNodesAccountedFor verifies that every original
// node (0-33) appears in at least one community after the full pipeline.
func TestPersonaGraphKarateClub_AllNodesAccountedFor(t *testing.T) {
	g := buildGraph(testdata.KarateClubEdges)

	local := NewLouvain(LouvainOptions{Seed: 42})
	personaGraph, _, inverseMap, err := buildPersonaGraph(g, local)
	if err != nil {
		t.Fatalf("buildPersonaGraph error: %v", err)
	}

	global := NewLouvain(LouvainOptions{Seed: 42})
	globalResult, err := global.Detect(personaGraph)
	if err != nil {
		t.Fatalf("global Detect error: %v", err)
	}

	nodeCommunities := mapPersonasToOriginal(globalResult.Partition, inverseMap)

	// Every node 0-33 must map to at least one community.
	missing := []int{}
	for i := 0; i < 34; i++ {
		comms, ok := nodeCommunities[NodeID(i)]
		if !ok || len(comms) == 0 {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		t.Errorf("nodes missing from community assignments: %v", missing)
	}
}
