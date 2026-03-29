# Phase 04: Performance Hardening + Benchmark Fixtures - Context

**Gathered:** 2026-03-29
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase — no user-facing behavior)

<domain>
## Phase Boundary

Performance hardening of Louvain and Leiden algorithms to meet <100ms/10K-node target, race-free concurrent use via sync.Pool integration, and accuracy validation on three benchmark graphs (Karate Club, Football 115-node, Polbooks 105-node). No new user-facing behavior — purely internal algorithmic and test infrastructure.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase. Key constraints:
- Must hit `< 100ms/op` on `BenchmarkLouvain10K` and `BenchmarkLeiden10K`
- `sync.Pool` for state allocation — 0 allocs/op on repeated same-size calls
- `neighborWeightBuf` dirty-list trick for O(1) reset
- Race-free: `go test -race ./graph/...` must pass with zero reports
- Football (115-node) and Polbooks (105-node) fixtures added to testdata
- NMI validation on all three benchmark graphs
- All 8 edge cases pass (already done in Phase 03 for Leiden; verify Louvain coverage)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `graph/louvain.go` — full Louvain implementation (phase1, deltaQ, buildSupergraph, normalizePartition, reconstructPartition)
- `graph/leiden.go` — full Leiden implementation with refinePartition (BFS)
- `graph/leiden_state.go` — leidenState struct pattern (reference for louvainState pooling)
- `graph/testdata/karate.go` — KarateClubEdges, KarateClubPartition fixtures
- `graph/leiden_test.go` — nmi() helper, TestLeidenKarateClubAccuracy pattern (reuse for Football/Polbooks)

### Established Patterns
- Partition: `map[NodeID]int`
- State allocation: struct with seed-based RNG, communityStrengths map
- Tests: table-driven, ground-truth comparison via NMI
- Commits: atomic per task, --no-verify in parallel

### Integration Points
- `graph/louvain_state.go` (if it exists) or inline `louvainState` in louvain.go — add sync.Pool here
- `graph/testdata/` — add football.go, polbooks.go fixtures
- `graph/benchmark_test.go` — new file for Go benchmark functions

</code_context>

<specifics>
## Specific Ideas

- Two plans as indicated in ROADMAP:
  1. Football + Polbooks fixtures, NMI helper reuse, accuracy test suite
  2. sync.Pool integration, neighborWeightBuf dirty-list, benchmarks + benchstat baseline
- neighborWeightBuf dirty-list: maintain a `[]NodeID` list of touched neighbors; reset only those entries instead of clearing entire map
- 10K-node synthetic graph: random Erdős–Rényi or Barabási–Albert graph generated in benchmark setup

</specifics>

<deferred>
## Deferred Ideas

None — discuss phase skipped (infrastructure phase).

</deferred>
