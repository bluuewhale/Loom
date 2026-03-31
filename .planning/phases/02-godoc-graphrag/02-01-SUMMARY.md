---
phase: 02-godoc-graphrag
plan: "01"
subsystem: graph
tags: [docs, examples, godoc, graphrag, readme]
dependency_graph:
  requires: []
  provides: [graph/example_test.go, README API accuracy]
  affects: [README.md, graph/example_test.go]
tech_stack:
  added: []
  patterns: [Go testable examples (package graph_test)]
key_files:
  created:
    - graph/example_test.go
  modified:
    - README.md
decisions:
  - Use package graph_test (external test package) for example tests to mirror real caller experience
  - uniqueCommunities helper defined locally in example_test.go (not exported from main package)
  - NMI table values taken from live test runs (Louvain KarateClub=0.83, Football/Polbooks=1.000)
metrics:
  duration: "~3min"
  completed: "2026-03-30"
  tasks: 2
  files_changed: 2
---

# Phase 02 Plan 01: godoc-graphrag — Example Tests and README Fixes Summary

Godoc-runnable example tests for the three primary constructors plus README API corrections and a new GraphRAG Example section.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | Create graph/example_test.go | 827814f | graph/example_test.go (created) |
| 2 | Fix README API errors and add GraphRAG Example section | fc646ac | README.md (modified) |

## What Was Built

**Task 1 — graph/example_test.go:**
- `ExampleNewLouvain`: two-cluster graph, asserts 2 communities detected and Q > 0
- `ExampleNewLeiden`: same topology via Leiden, asserts 2 communities and Q > 0
- `ExampleNewRegistry`: string-label graph using `NewRegistry`/`Register`/`Name`, verifies alice/bob/carol co-cluster and name round-trip
- All three pass `go test ./graph/... -run Example` and `go vet ./graph/...`

**Task 2 — README.md fixes (Rule 1 — Bug):**

API corrections:
- `NewNodeRegistry()` → `NewRegistry()`
- `reg.Add()` → `reg.Register()`
- `reg.Label(id)` → `reg.Name(id)` (returns `(string, bool)`)
- `LeidenOptions.MaxPasses` → `MaxIterations`
- Added `Len()` to NodeRegistry API table

NMI table corrections (values from live test runs):
- Louvain KarateClub: `0.65+` → `0.83` (actual seed=1 result)
- Political Books: `—` → `1.000` for both algorithms
- College Football: `—` → `1.000` for both algorithms

New GraphRAG Example section added between Performance and "When to use" sections:
- Similarity-graph clustering pipeline (static case)
- Warm-start online pipeline (incremental updates)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] README API names didn't match actual exported symbols**
- **Found during:** Task 2 (pre-task API audit)
- **Issue:** README showed `NewNodeRegistry`, `Add`, `Label`, `MaxPasses` — none of which exist; actual exports are `NewRegistry`, `Register`, `Name`, `MaxIterations`
- **Fix:** Corrected all four mismatches plus added the `Len()` method that was missing from the table
- **Files modified:** README.md
- **Commit:** fc646ac

**2. [Rule 1 - Bug] NMI accuracy table had placeholder/wrong values**
- **Found during:** Task 2 (ran accuracy tests to get real numbers)
- **Issue:** Louvain KarateClub showed `0.65+` (conservative lower-bound estimate); Football and Polbooks showed `—` (unpopulated)
- **Fix:** Replaced with measured values from `go test -v -run NMI`
- **Files modified:** README.md
- **Commit:** fc646ac

## Known Stubs

None.

## Self-Check: PASSED

- [x] graph/example_test.go exists and all three Example functions pass
- [x] go vet ./graph/... clean
- [x] Commit 827814f exists (example_test.go)
- [x] Commit fc646ac exists (README.md)
- [x] README NodeRegistry API matches registry.go exports
- [x] README LeidenOptions.MaxIterations matches detector.go
- [x] NMI values match live test output
