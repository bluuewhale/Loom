package graph

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/bluuewhale/loom/graph/testdata"
)

// generateBA builds an undirected Barabasi-Albert preferential attachment graph
// with n nodes and m new edges per added node. Uses a seeded RNG for reproducibility.
// Start with a complete graph on m nodes, then add nodes n=m..n-1 each connecting
// to m existing nodes chosen with probability proportional to degree.
func generateBA(n, m int, seed int64) *Graph {
	rng := rand.New(rand.NewSource(seed))
	g := NewGraph(false)

	// Seed with a complete graph on m nodes (IDs 0..m-1).
	for i := 0; i < m; i++ {
		for j := i + 1; j < m; j++ {
			g.AddEdge(NodeID(i), NodeID(j), 1.0)
		}
	}

	// Degree list for preferential attachment: each node appears once per edge endpoint.
	// For an undirected edge (u,v), both u and v are added.
	degList := make([]int, 0, 2*m*m)
	for i := 0; i < m; i++ {
		for j := 0; j < m-1; j++ {
			degList = append(degList, i)
		}
	}

	// Add remaining nodes n=m..n-1.
	for newNode := m; newNode < n; newNode++ {
		// Choose m distinct targets by preferential attachment.
		chosen := make(map[int]struct{}, m)
		targets := make([]int, 0, m)
		for len(targets) < m {
			idx := rng.Intn(len(degList))
			t := degList[idx]
			if _, dup := chosen[t]; !dup && t != newNode {
				chosen[t] = struct{}{}
				targets = append(targets, t)
			}
		}
		for _, t := range targets {
			g.AddEdge(NodeID(newNode), NodeID(t), 1.0)
			// Extend degree list with both endpoints.
			degList = append(degList, newNode, t)
		}
	}
	return g
}

// bench10K is the shared 10K-node BA graph for all benchmarks.
// Initialized once at package load; all benchmarks share the same pointer (read-only).
var bench10K *Graph

func init() {
	bench10K = generateBA(10_000, 5, 42)
}

// BenchmarkLouvain10K measures Louvain on a 10K-node Barabasi-Albert graph.
// Target: < 100ms/op. Run with -benchmem to see alloc counts.
func BenchmarkLouvain10K(b *testing.B) {
	det := NewLouvain(LouvainOptions{Seed: 1})
	det.Detect(bench10K) // warmup: populate sync.Pool
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench10K)
	}
}

// BenchmarkLeiden10K measures Leiden on a 10K-node Barabasi-Albert graph.
// Target: < 100ms/op. Run with -benchmem to see alloc counts.
func BenchmarkLeiden10K(b *testing.B) {
	det := NewLeiden(LeidenOptions{Seed: 1})
	det.Detect(bench10K) // warmup: populate sync.Pool
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench10K)
	}
}

// BenchmarkLouvain10K_Allocs is identical to BenchmarkLouvain10K but exists as a
// named fixture so benchstat can track allocation counts independently.
func BenchmarkLouvain10K_Allocs(b *testing.B) {
	det := NewLouvain(LouvainOptions{Seed: 1})
	det.Detect(bench10K) // warmup
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench10K)
	}
}

// BenchmarkLouvainWarmStart measures warm-start Louvain on a perturbed 10K-node graph.
// Setup: cold detect on bench10K, perturb +-100 edges (~1%), then warm detect.
// Target: warm ns/op <= 50% of BenchmarkLouvain10K ns/op.
func BenchmarkLouvainWarmStart(b *testing.B) {
	// Setup: cold detect to get prior partition
	det := NewLouvain(LouvainOptions{Seed: 1})
	coldResult, err := det.Detect(bench10K)
	if err != nil {
		b.Fatalf("cold detect: %v", err)
	}

	// Perturb: remove 100 + add 100 edges (~1% of ~50K edges)
	perturbed := perturbGraph(bench10K, 100, 100, 42)

	// Warm detector
	warmDet := NewLouvain(LouvainOptions{Seed: 1, InitialPartition: coldResult.Partition})
	warmDet.Detect(perturbed) // warmup: populate sync.Pool

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		warmDet.Detect(perturbed)
	}
}

// BenchmarkLeidenWarmStart measures warm-start Leiden on a perturbed 10K-node graph.
// Setup: cold detect on bench10K, perturb +-100 edges (~1%), then warm detect.
// Target: warm ns/op <= 50% of BenchmarkLeiden10K ns/op.
func BenchmarkLeidenWarmStart(b *testing.B) {
	// Setup: cold detect to get prior partition
	det := NewLeiden(LeidenOptions{Seed: 1})
	coldResult, err := det.Detect(bench10K)
	if err != nil {
		b.Fatalf("cold detect: %v", err)
	}

	// Perturb: remove 100 + add 100 edges (~1% of ~50K edges)
	perturbed := perturbGraph(bench10K, 100, 100, 42)

	// Warm detector
	warmDet := NewLeiden(LeidenOptions{Seed: 1, InitialPartition: coldResult.Partition})
	warmDet.Detect(perturbed) // warmup: populate sync.Pool

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		warmDet.Detect(perturbed)
	}
}

// TestConcurrentDetect verifies that concurrent Detect calls on distinct *Graph
// instances produce no data races. Run with -race flag to catch violations. (PERF-02)
func TestConcurrentDetect(t *testing.T) {
	// Build 4 distinct Karate Club graph instances (separate allocations).
	graphs := make([]*Graph, 4)
	for i := range graphs {
		graphs[i] = buildGraph(testdata.KarateClubEdges)
	}

	var wg sync.WaitGroup
	for i, g := range graphs {
		wg.Add(1)
		go func(g *Graph, idx int) {
			defer wg.Done()
			detL := NewLouvain(LouvainOptions{Seed: int64(idx + 1)})
			detLd := NewLeiden(LeidenOptions{Seed: int64(idx + 1)})
			for j := 0; j < 10; j++ {
				_, err := detL.Detect(g)
				if err != nil {
					t.Errorf("goroutine %d Louvain Detect error: %v", idx, err)
					return
				}
				_, err = detLd.Detect(g)
				if err != nil {
					t.Errorf("goroutine %d Leiden Detect error: %v", idx, err)
					return
				}
			}
		}(g, i)
	}
	wg.Wait()
}
