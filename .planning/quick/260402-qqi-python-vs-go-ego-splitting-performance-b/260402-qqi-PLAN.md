---
phase: quick
plan: 260402-qqi
type: execute
wave: 1
depends_on: []
files_modified:
  - benchmarks/ego_splitting_bench.py
  - graph/benchmark_test.go
  - benchmarks/ego_splitting_results.md
autonomous: true
requirements: []
must_haves:
  truths:
    - "Python benchmark runs EgoNetSplitter on BA(1K,5,42) and BA(10K,5,42) with timing"
    - "Go benchmark includes BenchmarkEgoSplitting10K_UnlimitedPasses (MaxPasses=0)"
    - "Results markdown contains side-by-side comparison with median/min/max timing"
  artifacts:
    - path: "benchmarks/ego_splitting_bench.py"
      provides: "Python EgoNetSplitter benchmark script"
    - path: "graph/benchmark_test.go"
      provides: "Go benchmark with unlimited passes variant"
      contains: "BenchmarkEgoSplitting10K_UnlimitedPasses"
    - path: "benchmarks/ego_splitting_results.md"
      provides: "Side-by-side results comparison"
  key_links: []
---

<objective>
Benchmark Python karateclub EgoNetSplitter vs Loom Go EgoSplitting on identical BA graphs, producing a results comparison table.

Purpose: Quantify Go vs Python performance difference for ego-splitting community detection.
Output: Python benchmark script, Go unlimited-passes benchmark variant, results markdown with timing comparison.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@graph/benchmark_test.go
@.planning/quick/260402-qqi-python-vs-go-ego-splitting-performance-b/260402-qqi-RESEARCH.md
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create Python benchmark and Go unlimited-passes variant</name>
  <files>benchmarks/ego_splitting_bench.py, graph/benchmark_test.go</files>
  <action>
1. Create `benchmarks/` directory.

2. Install karateclub: `pip install karateclub` (or `pip3`). Verify import works.

3. Create `benchmarks/ego_splitting_bench.py`:
   - Shebang: `#!/usr/bin/env python3`
   - Test sizes: n=1000 and n=10000, m=5, seed=42
   - Use `nx.barabasi_albert_graph(n, m, seed=seed)` for graph generation
   - Use `karateclub.EgoNetSplitter(resolution=1.0)`
   - Warmup: call `splitter.fit(g)` once before timing
   - Timing: `timeit.repeat(lambda: splitter.fit(g), number=1, repeat=10)`
   - Report: median, min, max in ms
   - Report: community count = `len(set(c for v in memberships.values() for c in v))`
   - Report: overlapping node count = `sum(1 for v in memberships.values() if len(v) > 1)`
   - Print format: `n=NNNNN: median=XXX.Xms min=XXX.Xms max=XXX.Xms communities=N overlapping_nodes=N/N`
   - Import time must NOT be included in timing (imports at top, timeit.repeat measures only fit())

4. Add `BenchmarkEgoSplitting10K_UnlimitedPasses` to `graph/benchmark_test.go`:
   - Place it immediately after the existing `BenchmarkEgoSplitting10K` function
   - Pattern: identical to `BenchmarkEgoSplitting10K` but GlobalDetector uses `NewLouvain(LouvainOptions{Seed: 1})` with NO MaxPasses (defaults to 0 = unlimited)
   - LocalDetector: `NewLouvain(LouvainOptions{Seed: 1})` (same as existing)
   - Warmup call before b.ResetTimer(), b.ReportAllocs()
   - This variant matches Python's `community.best_partition` which always runs to convergence

Also add `BenchmarkEgoSplitting1K` for the 1K comparison point:
   - Same pattern as 10K but uses `bench1K`
   - GlobalDetector with `MaxPasses: 1` (production setting)
  </action>
  <verify>
    <automated>cd /Users/donghyungko/.superset/worktrees/Loom/feat/auto-optimize && go build ./graph/... && python3 -c "from karateclub import EgoNetSplitter; print('OK')"</automated>
  </verify>
  <done>Python script exists at benchmarks/ego_splitting_bench.py, Go benchmark_test.go compiles with new benchmark functions</done>
</task>

<task type="auto">
  <name>Task 2: Run both benchmarks and produce results comparison</name>
  <files>benchmarks/ego_splitting_results.md</files>
  <action>
1. Run Go benchmarks (all EgoSplitting variants):
   ```
   go test ./graph/... -bench=BenchmarkEgoSplitting -benchmem -count=5 -timeout=600s
   ```
   Capture output. Extract ns/op, allocs/op, bytes/op for each variant.

2. Run Python benchmark:
   ```
   python3 benchmarks/ego_splitting_bench.py
   ```
   Capture output. Extract median/min/max ms and community/overlap counts.

3. Also run Go 1K benchmark for comparison:
   ```
   go test ./graph/... -bench=BenchmarkEgoSplitting1K -benchmem -count=5
   ```

4. Write `benchmarks/ego_splitting_results.md` containing:

   - Header: "# Ego-Splitting Performance: Go (Loom) vs Python (karateclub)"
   - Date and machine info (from `uname -m`, `go version`, `python3 --version`)
   - Graph description: BA(n, m=5, seed=42), unweighted
   - Results table for n=1000:
     | Implementation | Median (ms) | Min (ms) | Max (ms) | Communities | Overlapping Nodes |
   - Results table for n=10000:
     | Implementation | Median (ms) | Min (ms) | Max (ms) | Communities | Overlapping Nodes |
     Include rows for: Go (MaxPasses=1), Go (Unlimited), Python
   - Speedup summary: Go median / Python median ratio for each size
   - Go memory stats: allocs/op, bytes/op
   - Notes section explaining MaxPasses=1 vs unlimited difference

   Convert Go bench ns/op to ms for comparison. For Go median, use the middle value from 5 runs (Go bench reports average per run, so use that directly as the representative value).
  </action>
  <verify>
    <automated>test -f /Users/donghyungko/.superset/worktrees/Loom/feat/auto-optimize/benchmarks/ego_splitting_results.md && head -5 /Users/donghyungko/.superset/worktrees/Loom/feat/auto-optimize/benchmarks/ego_splitting_results.md</automated>
  </verify>
  <done>benchmarks/ego_splitting_results.md exists with timing comparison tables showing Go vs Python performance for both graph sizes, including speedup ratios</done>
</task>

</tasks>

<verification>
- `python3 benchmarks/ego_splitting_bench.py` runs without error and prints timing for n=1000 and n=10000
- `go test ./graph/... -bench=BenchmarkEgoSplitting -benchmem -count=1` runs all EgoSplitting benchmarks
- `benchmarks/ego_splitting_results.md` contains comparison tables with actual measured values
</verification>

<success_criteria>
- Python benchmark produces timing data for both graph sizes
- Go benchmark includes both MaxPasses=1 and unlimited variants
- Results markdown shows clear performance comparison with speedup ratios
- All numbers are from actual benchmark runs, not estimates
</success_criteria>

<output>
After completion, create `.planning/quick/260402-qqi-python-vs-go-ego-splitting-performance-b/260402-qqi-SUMMARY.md`
</output>
