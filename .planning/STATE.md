# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-29)

**Core value:** 개발자가 GraphRAG 파이프라인을 Go로 구현할 때 필요한 그래프 알고리즘을 교체 가능한 인터페이스로 빠르게 가져다 쓸 수 있어야 한다.
**Current focus:** Phase 02 — Interface + Louvain Core

## Current Position

Phase: 02 of 04 (Interface + Louvain Core)
Plan: 0 of 2 in current phase
Status: Ready to plan
Last activity: 2026-03-29 — Roadmap created; Phase 01 complete (graph, modularity, registry)

Progress: [██░░░░░░░░] 20% (Phase 01 complete, 3/5 total plans done)

## Performance Metrics

**Velocity:**
- Total plans completed: 3
- Average duration: unknown
- Total execution time: unknown

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 01 | 3/3 | - | - |

**Recent Trend:**
- Last 5 plans: unknown
- Trend: Stable

*Updated after each plan completion*

## Accumulated Context

### Decisions

- [Phase 01]: Single `package graph` — no sub-packages; all types in one package
- [Phase 01]: `map[NodeID]int` as Partition — no external type, zero-alloc swap
- [Phase 01]: `NodeRegistry` optional — integer ID path stays available for perf-critical callers
- [Roadmap]: `CommunityDetector` interface with `Detect(g *Graph) (CommunityResult, error)` — swappable contract

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 02]: Verify `g.Edges()` API exists in graph.go before writing Louvain (CRIT-02 totalWeight fix depends on unique-edge iteration)
- [Phase 02]: Directed graph ΔQ formula not yet estimated — flag for Phase 02 planning; may defer directed support to v2

## Session Continuity

Last session: 2026-03-29
Stopped at: Roadmap written; Phase 02 ready to plan
Resume file: None
