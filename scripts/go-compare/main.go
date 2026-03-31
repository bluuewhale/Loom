// go-compare benchmarks multiple Go community-detection libraries on a 10K-node
// random graph and reports wall-clock time so results can be compared directly
// in the README Performance table.
//
// Libraries benchmarked:
//   - gonum community.Modularize (Louvain)
//   - github.com/ledyba/go-louvain (Louvain via NextLevel)
//   - github.com/vsuryav/leiden-go (Leiden) — skipped: infinite loop bug on large graphs
//
// Usage:
//
//	go run . [--runs N]
package main

import (
	"flag"
	"fmt"
	randv2 "math/rand/v2"
	"strconv"
	"time"

	golouvain "github.com/ledyba/go-louvain/louvain"
	leiden "github.com/vsuryav/leiden-go"
	"gonum.org/v1/gonum/graph/community"
	"gonum.org/v1/gonum/graph/simple"
)

const n = 10_000
const avgDegree = 10

// buildEdges returns a slice of [2]int edges for a random undirected graph.
func buildEdges(seed uint64) [][2]int {
	rng := randv2.New(randv2.NewPCG(seed, 0))
	edgesNeeded := n * avgDegree / 2
	edges := make([][2]int, 0, edgesNeeded)
	for e := 0; e < edgesNeeded; e++ {
		u := rng.IntN(n)
		v := rng.IntN(n)
		if u == v {
			continue
		}
		edges = append(edges, [2]int{u, v})
	}
	return edges
}

// benchGonum runs gonum community.Modularize and returns avg duration.
func benchGonum(edges [][2]int, runs int) time.Duration {
	g := simple.NewUndirectedGraph()
	for i := 0; i < n; i++ {
		g.AddNode(simple.Node(i))
	}
	for _, e := range edges {
		g.SetEdge(g.NewEdge(simple.Node(e[0]), simple.Node(e[1])))
	}

	fmt.Printf("  [gonum] graph: %d nodes, %d edges\n", g.Nodes().Len(), g.Edges().Len())

	// Warm up
	_ = community.Modularize(g, 1.0, randv2.NewPCG(0, 0))

	var total time.Duration
	var nComm int
	for i := 0; i < runs; i++ {
		src := randv2.NewPCG(uint64(i+1), 0)
		start := time.Now()
		result := community.Modularize(g, 1.0, src)
		elapsed := time.Since(start)
		total += elapsed
		nComm = len(result.Communities())
		fmt.Printf("    run %d: %v  (%d communities)\n", i+1, elapsed, nComm)
	}
	avg := total / time.Duration(runs)
	fmt.Printf("  gonum Louvain 10K nodes: avg %v over %d runs (%d communities)\n\n", avg, runs, nComm)
	return avg
}

// benchGoLouvain runs ledyba/go-louvain via NextLevel.
func benchGoLouvain(edges [][2]int, runs int) time.Duration {
	adj := make([]map[int]int, n)
	for i := range adj {
		adj[i] = make(map[int]int)
	}
	for _, e := range edges {
		u, v := e[0], e[1]
		adj[u][v]++
		adj[v][u]++
	}

	makeGraph := func() *golouvain.Graph {
		g := golouvain.MakeNewGraph(n, func(cs []*golouvain.Node) interface{} { return nil })
		g.Total = len(edges)
		for i := 0; i < n; i++ {
			node := &g.Nodes[i]
			node.Links = make([]golouvain.Link, 0, len(adj[i]))
			for to, w := range adj[i] {
				node.Links = append(node.Links, golouvain.Link{To: to, Weight: w})
				node.Degree += w
			}
		}
		return g
	}

	fmt.Printf("  [go-louvain] graph: %d nodes, %d edges\n", n, len(edges))

	// runToConvergence repeatedly calls NextLevel until the community count stops shrinking.
	runToConvergence := func() (time.Duration, int) {
		g := makeGraph()
		start := time.Now()
		for {
			next := g.NextLevel(0, 0.0001)
			if len(next.Nodes) >= len(g.Nodes) {
				break
			}
			g = next
		}
		return time.Since(start), len(g.Nodes)
	}

	// Warm up
	_, _ = runToConvergence()

	var total time.Duration
	var nComm int
	for i := 0; i < runs; i++ {
		elapsed, communities := runToConvergence()
		total += elapsed
		nComm = communities
		fmt.Printf("    run %d: %v  (%d communities)\n", i+1, elapsed, nComm)
	}
	avg := total / time.Duration(runs)
	fmt.Printf("  go-louvain 10K nodes: avg %v over %d runs (%d communities)\n\n", avg, runs, nComm)
	return avg
}

// probeLeidenGo runs a single timed Leiden call with a 10s timeout to detect hangs.
// Returns (duration, true) if it completed, or (0, false) if it timed out.
func probeLeidenGo(edges [][2]int) (time.Duration, bool) {
	edgeMap := make(map[string]map[string]float64, n)
	for i := 0; i < n; i++ {
		edgeMap[strconv.Itoa(i)] = make(map[string]float64)
	}
	for _, e := range edges {
		u := strconv.Itoa(e[0])
		v := strconv.Itoa(e[1])
		edgeMap[u][v] += 1.0
		edgeMap[v][u] += 1.0
	}
	g := leiden.NewGraph(edgeMap)
	cfg := leiden.DefaultConfig()

	type res struct {
		dur time.Duration
		err error
	}
	ch := make(chan res, 1)
	go func() {
		start := time.Now()
		_, err := leiden.Leiden(g, cfg)
		ch <- res{time.Since(start), err}
	}()

	select {
	case r := <-ch:
		return r.dur, r.err == nil
	case <-time.After(10 * time.Second):
		return 0, false
	}
}

func main() {
	runs := flag.Int("runs", 5, "number of timed runs after warm-up")
	flag.Parse()

	edges := buildEdges(42)
	fmt.Printf("=== go-compare: community detection benchmarks, 10K nodes ===\n\n")

	fmt.Println("--- gonum (Louvain) ---")
	gonumAvg := benchGonum(edges, *runs)

	fmt.Println("--- go-louvain (ledyba/go-louvain) ---")
	goLouvainAvg := benchGoLouvain(edges, *runs)

	fmt.Println("--- leiden-go (vsuryav/leiden-go) — probing for hang ---")
	fmt.Printf("  [leiden-go] graph: %d nodes, %d edges\n", n, len(edges))
	leidenDur, leidenOK := probeLeidenGo(edges)
	if leidenOK {
		fmt.Printf("  leiden-go completed in %v\n\n", leidenDur)
	} else {
		fmt.Printf("  leiden-go: TIMED OUT after 10s — infinite loop in refinePartition\n")
		fmt.Printf("  (bug: refinePartition sets improved=true unconditionally on disconnected\n")
		fmt.Printf("   communities; outer loop never converges on large random graphs)\n\n")
	}

	fmt.Println("=== Summary ===")
	fmt.Printf("  gonum Louvain:   %v\n", gonumAvg)
	fmt.Printf("  go-louvain:      %v\n", goLouvainAvg)
	if leidenOK {
		fmt.Printf("  leiden-go:       %v\n", leidenDur)
	} else {
		fmt.Printf("  leiden-go:       N/A (infinite loop bug — skipped)\n")
	}
}
