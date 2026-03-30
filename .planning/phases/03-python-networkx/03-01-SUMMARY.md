---
phase: "03"
plan: "01"
subsystem: benchmarks
tags: [benchmark, python, networkx, comparison, performance]
dependency_graph:
  requires: [graph/benchmark_test.go, bench-baseline.txt]
  provides: [scripts/compare.py, BenchmarkLouvain1K, BenchmarkLeiden1K]
  affects: [README.md]
tech_stack:
  added: [scripts/compare.py (Python 3, NetworkX)]
  patterns: [BA graph fixture at 1K scale, Go vs Python benchmark table]
key_files:
  created:
    - scripts/compare.py
  modified:
    - graph/benchmark_test.go
    - bench-baseline.txt
    - README.md
decisions:
  - "bench1K uses generateBA(1_000, 5, 42) — identical parameters to bench10K, only n differs"
  - "compare.py reads Go baselines from bench-baseline.txt with Apple M4 fallback defaults"
  - "NetworkX Leiden omitted from table — NX 3.x Leiden is a dispatch stub, not implemented"
metrics:
  duration: "9 min"
  completed: "2026-03-30"
  tasks_completed: 4
  files_modified: 4
---

# Phase 03 Plan 01: Python NetworkX Benchmark Comparison Summary

**One-liner:** Added 1K-node Go benchmarks and `scripts/compare.py` showing ~12x Louvain speedup over Python NetworkX on Apple M4.

## What Was Built

1. **`BenchmarkLouvain1K` + `BenchmarkLeiden1K`** — new benchmark functions in `graph/benchmark_test.go` using a shared `bench1K` BA graph (1K nodes, m=5, seed=42). Results: Louvain ~5.4ms/op, Leiden ~5.8ms/op.

2. **`bench-baseline.txt` updated** — 1K results (5 runs each, Apple M4 arm64) prepended to the existing 10K baselines.

3. **`scripts/compare.py`** — runnable Python script that benchmarks NetworkX Louvain on the same 1K BA graph fixture and prints a side-by-side comparison table. Reads Go numbers from `bench-baseline.txt` with fallback defaults.

4. **`README.md` Performance section** — expanded to a Go vs Python table covering 1K and 10K, with note on NetworkX Leiden stub limitation and a `scripts/compare.py` reproduction command.

## Measured Numbers

| Graph size | Algorithm | Go (loom) | Python (NetworkX) | Speedup |
|------------|-----------|-----------|-------------------|---------|
| 1K nodes   | Louvain   | ~5.4ms    | ~65ms             | ~12x    |
| 1K nodes   | Leiden    | ~5.8ms    | N/A (stub)        | —       |
| 10K nodes  | Louvain   | ~60ms     | —                 | —       |
| 10K nodes  | Leiden    | ~65ms     | —                 | —       |

Hardware: Apple M4, arm64, Go 1.21+, NetworkX 3.6.1, Python 3.12.

## Deviations from Plan

None — plan was derived from success criteria; all criteria met as specified.

## Known Stubs

None.

## Self-Check: PASSED

- `graph/benchmark_test.go` — BenchmarkLouvain1K and BenchmarkLeiden1K present and passing
- `bench-baseline.txt` — 1K results present
- `scripts/compare.py` — file created and produces correct output
- `README.md` — Performance section has Go vs Python comparison table
- All existing tests pass: `go test ./graph/` — ok
