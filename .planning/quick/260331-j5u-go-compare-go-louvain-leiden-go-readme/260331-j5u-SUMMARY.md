---
phase: quick
plan: 260331-j5u
subsystem: scripts/go-compare, README
tags: [benchmark, comparison, go-louvain, leiden-go, performance]
dependency_graph:
  requires: []
  provides: [go-compare extended benchmark, README comparison table]
  affects: [README.md]
tech_stack:
  added:
    - github.com/ledyba/go-louvain v0.0.0-20220113123819-4f03491a0437
    - github.com/vsuryav/leiden-go v0.0.0-20251120005855-0f56599dc139
  patterns:
    - goroutine+channel timeout probe for library hang detection
key_files:
  created: []
  modified:
    - scripts/go-compare/main.go
    - scripts/go-compare/go.mod
    - scripts/go-compare/go.sum
    - README.md
decisions:
  - leiden-go skipped: refinePartition sets improved=true unconditionally on any disconnected community, creating an infinite outer loop on large random graphs
  - go-louvain noted as single-pass: NextLevel performs only one Louvain phase-1 sweep, producing ~4K communities vs loom's ~22 — not directly comparable
metrics:
  duration: 11min
  completed: 2026-03-31
  tasks_completed: 2
  files_modified: 4
---

# Quick Task 260331-j5u: go-compare go-louvain leiden-go README

**One-liner:** Extended go-compare benchmark to include go-louvain (~10ms, single-pass) and attempted leiden-go (skipped — infinite loop bug), with updated README comparison table.

## Tasks Completed

| # | Task | Commit | Files |
|---|------|--------|-------|
| 1 | Extend scripts/go-compare/ with go-louvain and leiden-go | f671f21 | scripts/go-compare/main.go, go.mod, go.sum |
| 2 | Update README Performance table | 6726c54 | README.md |

## Benchmark Results (real numbers)

| Library | Algorithm | Avg Time | Communities | Notes |
|---------|-----------|----------|-------------|-------|
| loom | Louvain | ~48ms | ~22 | from existing bench |
| loom | Leiden | ~57ms | ~22 | from existing bench |
| gonum | Louvain | ~2.3s | ~22 | `community.Modularize`, 5 runs |
| go-louvain | Louvain (1 pass) | ~9.8ms | ~4,300 | single `NextLevel` call |
| leiden-go | Leiden | N/A | N/A | infinite loop bug |

## Deviations from Plan

### Auto-fixed Issues

None — plan executed as written.

### Library Issues Found

**1. leiden-go — infinite loop on 10K-node graph**
- **Found during:** Task 1 benchmark run
- **Root cause:** `refinePartition()` in `leiden.go` sets `improved = true` unconditionally whenever any community fails `isWellConnected()`. On a random 10K graph with many isolated subgraphs, this condition never clears and the outer `for improved` loop runs forever.
- **Action taken:** Added 10-second goroutine+channel timeout probe; confirmed hang; skipped library and documented in README table and summary.
- **Library version:** `v0.0.0-20251120005855-0f56599dc139`

**2. go-louvain — single-level pass only**
- **Found during:** Task 1 benchmark run
- **Observation:** `NextLevel` performs only one Louvain phase-1 sweep (no supergraph compression loop). Produces ~4,300 communities vs loom's ~22, making timing non-comparable for quality-equivalent partitions. Speed (~10ms) is explained by doing far less work.
- **Action taken:** Noted in README table with explicit caveat; timing reported as-is.

## Known Stubs

None.

## Self-Check

- [x] scripts/go-compare/main.go exists and builds cleanly
- [x] scripts/go-compare/go.mod includes go-louvain and leiden-go
- [x] README.md Performance table has all 5 rows
- [x] Commits f671f21 and 6726c54 exist
