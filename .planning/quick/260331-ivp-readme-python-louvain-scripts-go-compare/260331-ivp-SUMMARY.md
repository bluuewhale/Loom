---
phase: quick
plan: 260331-ivp
subsystem: scripts, docs
tags: [benchmark, gonum, performance, readme]
dependency_graph:
  requires: []
  provides: [scripts/go-compare, README performance table]
  affects: [README.md]
tech_stack:
  added: [gonum.org/v1/gonum v0.17.0, math/rand/v2]
  patterns: [standalone Go benchmark module, wall-clock timing loop]
key_files:
  created:
    - scripts/go-compare/go.mod
    - scripts/go-compare/go.sum
    - scripts/go-compare/main.go
  modified:
    - README.md
decisions:
  - "Use community.Modularize (not a Louvain() function — gonum uses Modularize as the top-level entry point)"
  - "Use math/rand/v2 PCG source — gonum v0.17 uses rand/v2, rand/v1 Source incompatible"
  - "5-run timing loop with one warm-up discard — matches loom benchmark methodology"
  - "Footnote distinguishes networkx vs python-louvain — two separate libraries with incompatible APIs"
metrics:
  duration: 15min
  completed: 2026-03-31
  tasks: 2
  files: 4
---

# Quick Task 260331-ivp: go-compare module + README Performance table

Go benchmark module for gonum Louvain comparison, revealing ~46x speed advantage for loom (~50ms vs ~2.3s on 10K nodes), plus clarifying footnote distinguishing NetworkX from python-louvain library.

## Tasks Completed

| # | Name | Commit | Files |
|---|------|--------|-------|
| 1 | Create scripts/go-compare/ benchmark module | 456270b | scripts/go-compare/{go.mod,go.sum,main.go} |
| 2 | Update README Performance table + footnote | 11b5d93 | README.md |

## Benchmark Results (Apple M4, arm64)

Graph: 10K nodes, ~50K edges, random undirected

| Library | Algorithm | Time |
|---------|-----------|------|
| loom | Louvain | ~50ms |
| loom | Leiden | ~56ms |
| gonum/graph/community | Louvain | ~2.3s |

Individual gonum runs: 1.69s, 1.74s, 2.41s, 2.12s, 3.53s (avg 2.30s over 5 runs).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] gonum v0.17 uses math/rand/v2, not math/rand**

- **Found during:** Task 1 — first `go build` attempt
- **Issue:** `rand.NewSource(1)` returns `math/rand.Source` (v1) which does not implement `math/rand/v2.Source` (requires `Uint64()` method). gonum v0.17 upgraded to rand/v2.
- **Fix:** Changed import to `randv2 "math/rand/v2"` and used `randv2.NewPCG(seed, 0)` as the Source. Graph-building rng also switched to `randv2.New(randv2.NewPCG(42, 0))`.
- **Files modified:** scripts/go-compare/main.go
- **Commit:** 456270b

**2. [Rule 1 - Bug] gonum Louvain entry point is Modularize, not Louvain()**

- **Found during:** Task 1 — first `go build` attempt
- **Issue:** Initial code referenced `community.Louvain` and `community.ReducedUndirectedGraph` which do not exist as exported names. The public API is `community.Modularize(g, resolution, src) ReducedGraph`.
- **Fix:** Replaced `community.Louvain(...)` with `community.Modularize(g, 1.0, src)`, replaced `*community.ReducedUndirectedGraph` with `community.ReducedGraph` interface.
- **Files modified:** scripts/go-compare/main.go
- **Commit:** 456270b

## Known Stubs

None.

## Self-Check: PASSED

- scripts/go-compare/main.go: FOUND
- scripts/go-compare/go.mod: FOUND
- Commit 456270b: FOUND
- Commit 11b5d93: FOUND
