// go-compare benchmarks gonum community.Modularize (Louvain) on a 10K-node random graph.
//
// This script generates the same style of graph used by loom's own benchmarks
// (random undirected, ~10 avg degree) and reports wall-clock time per run so
// the numbers can be compared directly in the README Performance table.
//
// Usage:
//
//	go run . [--runs N]
package main

import (
	"flag"
	"fmt"
	randv2 "math/rand/v2"
	"time"

	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"
)

func main() {
	runs := flag.Int("runs", 5, "number of timed runs after warm-up")
	flag.Parse()

	const n = 10_000
	const avgDegree = 10

	// Build a random undirected graph with ~10K nodes, ~avgDegree edges per node.
	// Uses randv2 throughout so there is no dependency on math/rand v1.
	rng := randv2.New(randv2.NewPCG(42, 0))
	g := simple.NewUndirectedGraph()
	for i := 0; i < n; i++ {
		g.AddNode(simple.Node(i))
	}
	edgesNeeded := n * avgDegree / 2
	for e := 0; e < edgesNeeded; e++ {
		u := rng.IntN(n)
		v := rng.IntN(n)
		if u == v {
			continue
		}
		edge := g.NewEdge(simple.Node(u), simple.Node(v))
		g.SetEdge(edge)
	}

	nodeCount := g.Nodes().Len()
	edgeCount := g.Edges().Len()
	fmt.Printf("Graph: %d nodes, %d edges\n\n", nodeCount, edgeCount)

	// Warm up — discard to avoid cold-cache effects.
	_ = community.Modularize(g, 1.0, randv2.NewPCG(0, 0))

	var total time.Duration
	var nCommunities int
	for i := 0; i < *runs; i++ {
		src := randv2.NewPCG(uint64(i+1), 0)
		start := time.Now()
		result := community.Modularize(g, 1.0, src)
		elapsed := time.Since(start)
		total += elapsed
		comms := result.Communities()
		nCommunities = len(comms)
		fmt.Printf("  run %d: %v  (%d communities)\n", i+1, elapsed, nCommunities)
	}

	avg := total / time.Duration(*runs)
	fmt.Printf("\ngonum Louvain 10K nodes: avg %v over %d runs\n", avg, *runs)
	fmt.Printf("Communities found: %d\n", nCommunities)
}
