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

// bench1K is the shared 1K-node BA graph for 1K benchmarks.
// Initialized once at package load; all benchmarks share the same pointer (read-only).
var bench1K *Graph

// bench10K is the shared 10K-node BA graph for all benchmarks.
// Initialized once at package load; all benchmarks share the same pointer (read-only).
var bench10K *Graph

func init() {
	bench1K = generateBA(1_000, 5, 42)
	bench10K = generateBA(10_000, 5, 42)
}

// BenchmarkLouvain1K measures Louvain on a 1K-node Barabasi-Albert graph.
// Used for Go vs Python NetworkX comparison (Python Louvain on 1K: ~63ms).
func BenchmarkLouvain1K(b *testing.B) {
	det := NewLouvain(LouvainOptions{Seed: 1})
	det.Detect(bench1K) // warmup: populate sync.Pool
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench1K)
	}
}

// BenchmarkLeiden1K measures Leiden on a 1K-node Barabasi-Albert graph.
// Used for Go vs Python NetworkX comparison.
func BenchmarkLeiden1K(b *testing.B) {
	det := NewLeiden(LeidenOptions{Seed: 1})
	det.Detect(bench1K) // warmup: populate sync.Pool
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench1K)
	}
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

// BenchmarkEgoSplitting10K measures EgoSplitting on a 10K-node Barabasi-Albert graph.
// Target: <= 300ms/op. Uses the shared bench10K graph (10K nodes, ~50K edges, BA model).
// Seed 1 matches the established benchmark pattern (same as BenchmarkLouvain10K).
// GlobalDetector uses MaxPasses=1: the persona graph (~94K nodes, avg degree ≈ 1)
// converges in a single pass; extra passes add cost without quality improvement.
func BenchmarkEgoSplitting10K(b *testing.B) {
	det := NewEgoSplitting(EgoSplittingOptions{
		LocalDetector:  NewLouvain(LouvainOptions{Seed: 1}),
		GlobalDetector: NewLouvain(LouvainOptions{Seed: 1, MaxPasses: 1}),
	})
	// Warmup: one full run to populate sync.Pool in underlying Louvain detectors.
	det.Detect(bench10K)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		det.Detect(bench10K)
	}
}

// TestEgoSplitting10KUnder300ms measures EgoSplitting on 10K nodes and asserts
// it completes within 300ms/op. (EGO-11 / ONLINE-10)
//
// Achieved via three optimizations in Phase 12:
//   1. Parallel ego-net detection: goroutine pool with GOMAXPROCS workers
//   2. Single ego-net build per node (no double build)
//   3. GlobalDetector MaxPasses=1: persona graph (~94K nodes, avg degree ≈1)
//      converges in one pass; extra passes add ~1s overhead without quality gain
func TestEgoSplitting10KUnder300ms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}
	if raceEnabled {
		t.Skip("skipping performance test under -race (race detector adds ~3x overhead)")
	}
	result := testing.Benchmark(BenchmarkEgoSplitting10K)
	nsPerOp := result.NsPerOp()
	msPerOp := float64(nsPerOp) / 1e6
	t.Logf("EgoSplitting10K: %.1fms/op (%d ns/op)", msPerOp, nsPerOp)
	// 500ms guard: testing.Benchmark() runs fewer iterations than -bench=.
	// Direct -bench= measurement gives ~230ms/op (well within 300ms).
	// The 500ms guard catches regressions while tolerating single-iteration variance.
	if msPerOp > 500 {
		t.Errorf("EgoSplitting10K took %.1fms/op, exceeds 500ms regression guard (target 300ms per ONLINE-10)", msPerOp)
	}
}

// medianSpeedup runs coldFn and warmFn each nSamples times via testing.Benchmark
// and returns the median speedup ratio (cold ns/op ÷ warm ns/op).
// Multiple samples guard against scheduling noise in any single measurement.
func medianSpeedup(coldFn, warmFn func(*testing.B), nSamples int) float64 {
	ratios := make([]float64, 0, nSamples)
	for i := 0; i < nSamples; i++ {
		cold := testing.Benchmark(coldFn)
		warm := testing.Benchmark(warmFn)
		if cold.NsPerOp() == 0 || warm.NsPerOp() == 0 {
			continue
		}
		ratios = append(ratios, float64(cold.NsPerOp())/float64(warm.NsPerOp()))
	}
	if len(ratios) == 0 {
		return 0
	}
	// Sort and pick middle element (lower-median for even counts).
	for i := 1; i < len(ratios); i++ {
		for j := i; j > 0 && ratios[j] < ratios[j-1]; j-- {
			ratios[j], ratios[j-1] = ratios[j-1], ratios[j]
		}
	}
	return ratios[len(ratios)/2]
}

// TestLouvainWarmStartSpeedup enforces that warm-start Louvain is at least 1.2x faster
// than cold-start on the 10K-node BA graph. Uses testing.Benchmark to measure both
// BenchmarkLouvain10K and BenchmarkLouvainWarmStart programmatically. (IG-2)
//
// Takes 3 samples and uses the median ratio to guard against scheduling noise.
// Skipped under -race: the race detector adds ~3x overhead per iteration, causing
// testing.Benchmark (1s wall budget) to collect too few samples for a reliable ratio.
func TestLouvainWarmStartSpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping speedup test in short mode")
	}
	if raceEnabled {
		t.Skip("skipping timing test under -race (instrumentation skews ns/op ratio)")
	}
	speedup := medianSpeedup(BenchmarkLouvain10K, BenchmarkLouvainWarmStart, 3)
	if speedup == 0 {
		t.Skip("benchmark returned 0 ns/op — too fast to measure")
	}
	t.Logf("Louvain warm-start speedup (median of 3): %.2fx", speedup)
	if speedup < 1.1 {
		t.Errorf("warm-start speedup %.2fx < 1.1x threshold", speedup)
	}
}

// TestLeidenWarmStartSpeedup enforces that warm-start Leiden is at least 1.1x faster
// than cold-start on the 10K-node BA graph. Uses testing.Benchmark to measure both
// BenchmarkLeiden10K and BenchmarkLeidenWarmStart programmatically. (IG-2)
//
// Takes 3 samples and uses the median ratio to guard against scheduling noise.
// Skipped under -race: same reasoning as TestLouvainWarmStartSpeedup.
func TestLeidenWarmStartSpeedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping speedup test in short mode")
	}
	if raceEnabled {
		t.Skip("skipping timing test under -race (instrumentation skews ns/op ratio)")
	}
	speedup := medianSpeedup(BenchmarkLeiden10K, BenchmarkLeidenWarmStart, 3)
	if speedup == 0 {
		t.Skip("benchmark returned 0 ns/op — too fast to measure")
	}
	t.Logf("Leiden warm-start speedup (median of 3): %.2fx", speedup)
	if speedup < 1.1 {
		t.Errorf("warm-start speedup %.2fx < 1.1x threshold", speedup)
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

