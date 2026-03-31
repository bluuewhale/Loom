package graph_test

import (
	"fmt"
	"sort"

	"github.com/bluuewhale/loom/graph"
)

// ExampleNewLouvain demonstrates building a two-cluster graph and detecting
// communities with the Louvain algorithm. This pattern mirrors a GraphRAG
// pipeline: build a similarity graph over document chunks, detect communities,
// then use each community as a context window for an LLM.
func ExampleNewLouvain() {
	// Build a small undirected graph with two natural communities:
	// nodes {0,1,2} form one cluster and {3,4,5} form another,
	// connected by a single weak bridge edge.
	g := graph.NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 0, 1.0)
	g.AddEdge(3, 4, 1.0)
	g.AddEdge(4, 5, 1.0)
	g.AddEdge(5, 3, 1.0)
	g.AddEdge(2, 3, 0.1) // weak inter-cluster bridge

	det := graph.NewLouvain(graph.LouvainOptions{Seed: 42})
	result, err := det.Detect(g)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// Collect nodes per community, sorted for deterministic output.
	communities := make(map[int][]int)
	for node, comm := range result.Partition {
		communities[comm] = append(communities[comm], int(node))
	}
	for _, nodes := range communities {
		sort.Ints(nodes)
	}

	// Verify two communities were found.
	fmt.Println("communities:", len(communities))
	fmt.Println("modularity > 0:", result.Modularity > 0)

	// Output:
	// communities: 2
	// modularity > 0: true
}

// ExampleNewLeiden demonstrates building a two-cluster graph and detecting
// communities with the Leiden algorithm. Leiden guarantees connected
// communities via its BFS refinement step — valuable for GraphRAG pipelines
// where disconnected communities produce incoherent context windows.
func ExampleNewLeiden() {
	// Same two-cluster graph as ExampleNewLouvain.
	g := graph.NewGraph(false)
	g.AddEdge(0, 1, 1.0)
	g.AddEdge(1, 2, 1.0)
	g.AddEdge(2, 0, 1.0)
	g.AddEdge(3, 4, 1.0)
	g.AddEdge(4, 5, 1.0)
	g.AddEdge(5, 3, 1.0)
	g.AddEdge(2, 3, 0.1) // weak inter-cluster bridge

	det := graph.NewLeiden(graph.LeidenOptions{Seed: 42})
	result, err := det.Detect(g)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("communities:", len(communitySet(result.Partition)))
	fmt.Println("modularity > 0:", result.Modularity > 0)

	// Output:
	// communities: 2
	// modularity > 0: true
}

// ExampleNewRegistry demonstrates using NodeRegistry to build a graph with
// string node labels and translate results back to human-readable names.
// In GraphRAG pipelines, node labels typically correspond to document IDs,
// entity names, or chunk identifiers.
func ExampleNewRegistry() {
	reg := graph.NewRegistry()

	alice := reg.Register("alice")
	bob := reg.Register("bob")
	carol := reg.Register("carol")
	dave := reg.Register("dave")

	g := graph.NewGraph(false)
	g.AddEdge(alice, bob, 1.0)
	g.AddEdge(bob, carol, 1.0)
	g.AddEdge(carol, alice, 1.0)
	g.AddEdge(dave, carol, 0.1) // weak link from dave to the alice-bob-carol cluster

	det := graph.NewLouvain(graph.LouvainOptions{Seed: 1})
	result, err := det.Detect(g)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	// Verify alice, bob, carol end up in the same community.
	commAlice := result.Partition[alice]
	commBob := result.Partition[bob]
	commCarol := result.Partition[carol]
	fmt.Println("alice == bob community:", commAlice == commBob)
	fmt.Println("alice == carol community:", commAlice == commCarol)

	// Translate one node ID back to a label.
	label, ok := reg.Name(alice)
	fmt.Println("alice label:", label, ok)

	// Output:
	// alice == bob community: true
	// alice == carol community: true
	// alice label: alice true
}

// communitySet returns the set of distinct community IDs in a partition.
func communitySet(partition map[graph.NodeID]int) map[int]struct{} {
	seen := make(map[int]struct{})
	for _, c := range partition {
		seen[c] = struct{}{}
	}
	return seen
}
