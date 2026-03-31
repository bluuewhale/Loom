---
phase: quick
plan: 260331-id1
subsystem: graph
tags: [code-review, cleanup, benchmarks, tests]
dependency_graph:
  requires: []
  provides: [leiden-go-review-fixes]
  affects: [graph/leiden.go, graph/example_test.go, graph/leiden_numruns_test.go, graph/accuracy_test.go, scripts/compare.py, README.md, bench-baseline.txt]
tech_stack:
  added: []
  patterns: []
key_files:
  created: []
  modified:
    - graph/leiden.go
    - graph/example_test.go
    - graph/leiden_numruns_test.go
    - graph/accuracy_test.go
    - scripts/compare.py
    - README.md
    - bench-baseline.txt
decisions:
  - "communitySet rename applies only to example_test.go (package graph_test); internal testhelpers_test.go uniqueCommunities (package graph, returns int) is a separate function and was not renamed"
  - "bench-baseline.txt goos/pkg header added manually: Go 1.26.1 does not emit the header to stdout when output is redirected; header matches benchstat format from prior runs"
metrics:
  duration: 15min
  completed: 2026-03-31
---

# Quick Task 260331-id1: PR #4 Code Review Fixes Summary

**One-liner:** Apply 7 PR #4 code review items — silent-error comment, communitySet rename, NumRuns=2 test, NMI threshold tightening to 0.95, compare.py random_state, README timing fix, and bench-baseline.txt regeneration with correct module name.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Apply code and doc fixes (6 of 7 items) | f9b6352 | leiden.go, example_test.go, leiden_numruns_test.go, accuracy_test.go, compare.py, README.md |
| 2 | Regenerate bench-baseline.txt | f45a7d0 | bench-baseline.txt |

## Changes Applied

1. **graph/leiden.go** — Added 3-line comment above `return bestResult, nil` in `Detect()` explaining intentional `lastErr` discard when at least one multi-run iteration succeeds.

2. **graph/example_test.go** — Renamed `uniqueCommunities` function to `communitySet`; updated doc comment from "returns the number of distinct community IDs" to "returns the set of distinct community IDs"; updated call site on line 74.

3. **graph/leiden_numruns_test.go** — Added `TestLeidenNumRunsTwoExplicit` after `TestLeidenNumRunsOneIsEquivalent` with `NumRuns: 2`; asserts Q > 0 and Partition has 34 nodes; includes `t.Logf` showing Q and community count.

4. **graph/accuracy_test.go** — Tightened NMI thresholds from `0.5` to `0.95` for all 4 Football/Polbooks tests (`TestLouvainFootballNMI`, `TestLeidenFootballNMI`, `TestLouvainPolbooksNMI`, `TestLeidenPolbooksNMI`). Actual NMI values are 1.000 for all four, so the tighter threshold is correct.

5. **scripts/compare.py** — Added `random_state=42` to both `best_partition` calls in `_benchmark_louvain` (warmup call and timed lambda) for reproducible Python-side benchmarks.

6. **README.md** — Changed `~57ms` to `~56ms` in both occurrences (Features list line 10, Performance table line 150).

7. **bench-baseline.txt** — Regenerated with `go test -bench=. -benchmem -count=5 ./graph/...` on Apple M4 arm64. Added `goos/goarch/pkg/cpu` header manually (Go 1.26.1 does not emit this header to stdout when output is piped; header matches benchstat format). Module name updated from stale `community-detection/graph` to `github.com/bluuewhale/loom/graph`. New benchmarks included: `BenchmarkLouvainWarmStart`, `BenchmarkLeidenWarmStart`, `BenchmarkComputeModularityKarate`.

## Verification Results

All checks passed:

- `go test -count=1 ./graph/...` — PASS (6.003s)
- `grep "communitySet" graph/example_test.go` — function and call site present
- `grep "uniqueCommunities" graph/example_test.go` — empty (old name gone)
- `grep "random_state=42" scripts/compare.py` — both calls present
- `grep "~56ms" README.md` — 2 occurrences corrected
- `grep "pkg: github.com/bluuewhale/loom/graph" bench-baseline.txt` — OK

NMI results for tightened thresholds (Seed=42/2 deterministic):
- Football Louvain NMI=1.000, Football Leiden NMI=1.000
- Polbooks Louvain NMI=1.000, Polbooks Leiden NMI=1.000

## Deviations from Plan

None — plan executed exactly as written.

## Self-Check: PASSED

- f9b6352 exists: confirmed
- f45a7d0 exists: confirmed
- All 7 files modified as specified
- Full test suite passes
