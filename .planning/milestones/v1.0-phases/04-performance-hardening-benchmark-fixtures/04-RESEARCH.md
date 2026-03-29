# Phase 04: Performance Hardening + Benchmark Fixtures - Research

**Researched:** 2026-03-29
**Domain:** Go performance optimization (sync.Pool, benchmarks), graph algorithm accuracy testing, NMI validation
**Confidence:** HIGH

## Summary

Phase 04 is a pure internal-quality phase with two plans: (1) add Football and Polbooks benchmark fixtures and NMI accuracy tests, and (2) integrate `sync.Pool` for state reuse and add Go benchmark functions with a `benchstat` baseline. All work lives in `package graph`.

The existing codebase is well-structured for this work. `leidenState` and `louvainState` already exist as separate structs in `leiden_state.go` and `louvain_state.go`. The `nmi()` helper already lives in `leiden_test.go` and can be extracted to a shared test helper. All existing tests pass. `benchstat` is not installed yet — the plan must include a Wave 0 install step.

**Primary recommendation:** Extract `nmi()` to a package-internal test helper, add fixture `.go` files for Football and Polbooks, then add `sync.Pool` wrappers around `louvainState` and `leidenState` with the dirty-list reset trick for `neighborWeightBuf`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
All implementation choices are at Claude's discretion — pure infrastructure phase. Key constraints:
- Must hit `< 100ms/op` on `BenchmarkLouvain10K` and `BenchmarkLeiden10K`
- `sync.Pool` for state allocation — 0 allocs/op on repeated same-size calls
- `neighborWeightBuf` dirty-list trick for O(1) reset
- Race-free: `go test -race ./graph/...` must pass with zero reports
- Football (115-node) and Polbooks (105-node) fixtures added to testdata
- NMI validation on all three benchmark graphs
- All 8 edge cases pass (already done in Phase 03 for Leiden; verify Louvain coverage)

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase.

### Deferred Ideas (OUT OF SCOPE)
None — discuss phase skipped (infrastructure phase).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PERF-01 | 10K-node / ~50K-edge graph < 100ms single goroutine (`-bench` measured) | sync.Pool + dirty-list trick eliminates per-call allocations; 10K BA graph generation in `b.N` loop |
| PERF-02 | Concurrent-safe — different `*Graph` instances, concurrent `Detect`, no race | Each `Detect` call gets its own state from Pool; `*Graph` is read-only during detection |
| PERF-03 | `sync.Pool` for `louvainState` reuse — min allocations on repeated same-size calls | Standard Go sync.Pool pattern; Pool.Get + clear + Pool.Put |
| PERF-04 | `go test -race` passes with zero reports | Verified by `go test -race ./graph/...` command |
| TEST-01 | Karate Club — Louvain and Leiden both Q > 0.35 | Already passing for Louvain (Q=0.42) and Leiden (Q=0.37); add explicit Louvain NMI assertion |
| TEST-02 | Football network 115-node 613-edge fixture + NMI validation | Add `graph/testdata/football.go`; run both algorithms, assert NMI >= threshold |
| TEST-03 | Polbooks fixture 105-node 441-edge + NMI validation | Add `graph/testdata/polbooks.go`; run both algorithms, assert NMI >= threshold |
| TEST-04 | 8 edge cases — empty, single, disconnected, giant+singletons, 2-node, zero-resolution, complete, self-loop | Leiden has most; audit Louvain coverage; add missing cases |
| TEST-05 | `benchstat`-based performance regression baseline | Install `benchstat`, run `BenchmarkLouvain10K`/`BenchmarkLeiden10K`, save baseline file |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `sync` (stdlib) | Go 1.26.1 | `sync.Pool` for state recycling | Zero-dependency; purpose-built for exactly this use case |
| `math/rand` (stdlib) | Go 1.26.1 | RNG already used in state structs | Already in use |
| `testing` (stdlib) | Go 1.26.1 | Benchmarks (`testing.B`) | Official Go benchmark framework |
| `golang.org/x/perf/cmd/benchstat` | latest | Statistical benchmark comparison | Standard Go performance tooling |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `go test -race` | Go 1.26.1 built-in | Race detector | Run as part of CI gate |
| `go test -benchmem` | Go 1.26.1 built-in | Allocation tracking per op | Combined with `-bench` flag |

**Installation (benchstat):**
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

**Version verification:** Go 1.26.1 confirmed on target machine. No third-party library additions needed beyond benchstat.

## Architecture Patterns

### Recommended Project Structure
```
graph/
├── louvain_state.go        # Add sync.Pool + reset methods here
├── leiden_state.go         # Add sync.Pool + reset methods here
├── louvain.go              # Use pool in Detect()
├── leiden.go               # Use pool in Detect()
├── benchmark_test.go       # New: BenchmarkLouvain10K, BenchmarkLeiden10K
├── accuracy_test.go        # New: TestLouvainNMI, TestLeidenNMI for all 3 fixtures
├── testdata/
│   ├── karate.go           # Existing
│   ├── football.go         # New: 115-node fixture
│   └── polbooks.go         # New: 105-node fixture
```

### Pattern 1: sync.Pool for louvainState
**What:** Pool holds reusable `louvainState` objects. `Get()` returns an existing state or creates a new one via `New`. After use, `reset()` then `Put()` returns state to pool.
**When to use:** Any time `Detect` is called — eliminates map allocations on repeated same-size calls.

```go
// Source: https://pkg.go.dev/sync#Pool
var louvainPool = sync.Pool{
    New: func() any { return &louvainState{} },
}

// In Detect():
st := louvainPool.Get().(*louvainState)
st.init(g, seed)   // clear maps and reset, resize if needed
defer louvainPool.Put(st)
```

**Key constraint:** `sync.Pool` objects may be GC'd between calls. The pool is not a cache — it reduces allocation pressure but does not guarantee object reuse across GC cycles. This is acceptable: `0 allocs/op` is achievable within a single benchmark loop where GC is not triggered.

### Pattern 2: neighborWeightBuf dirty-list
**What:** Instead of `make(map[NodeID]float64)` per node per phase1 pass, maintain a `[]NodeID` (dirtyKeys) alongside the map. After computing ΔQ for a node, append touched keys to dirtyKeys. At end of node iteration, range over dirtyKeys to zero out map entries instead of replacing the map.

```go
// In louvainState / leidenState:
neighborBuf    map[NodeID]float64
neighborDirty  []NodeID  // indices into neighborBuf that were written this pass

// Reset in O(len(dirty)) instead of O(map capacity):
for _, k := range st.neighborDirty {
    delete(st.neighborBuf, k)
}
st.neighborDirty = st.neighborDirty[:0]  // reset slice without allocation
```

**When to use:** Inside phase1 local-move loop — called once per node per pass, so for 10K nodes × multiple passes this is the primary allocation hot-spot.

### Pattern 3: 10K Benchmark Graph Generation
**What:** Generate a synthetic Barabási-Albert or Erdős-Rényi graph in benchmark setup. The graph must have ~50K edges (5 edges/node average) to mirror realistic GraphRAG workloads.

```go
// Source: standard Go benchmark pattern
func BenchmarkLouvain10K(b *testing.B) {
    g := generateBA(10_000, 5, 42) // BA model: 10K nodes, m=5, seed=42
    det := NewLouvain(LouvainOptions{Seed: 1})
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        det.Detect(g)
    }
}
```

### Pattern 4: NMI Test Helper Extraction
**What:** `nmi()` currently lives in `leiden_test.go`. Both `louvain_test.go` and a new `accuracy_test.go` need it. Move to a shared test file (e.g., `testhelpers_test.go`) within `package graph`.

**Go rule:** Functions in `_test.go` files in the same package are accessible to all test files in that package. No export needed.

### Anti-Patterns to Avoid
- **Clearing neighborBuf with `make()` each iteration:** Causes heap allocation on every node; defeats pool purpose. Use dirty-list instead.
- **Storing `*Graph` in sync.Pool:** `*Graph` is user-provided and read-only; only algorithm state belongs in the pool.
- **Sharing Pool across algorithms:** Louvain and Leiden have different state structs; use separate pools.
- **b.StartTimer() inside inner loop:** Benchmark timer overhead; use `b.ResetTimer()` once after setup.
- **Forgetting `b.ReportAllocs()`:** Without `-benchmem` flag or explicit `ReportAllocs()`, allocation counts are not shown.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| State recycling | Custom free-list | `sync.Pool` | GC-integrated; automatic; zero maintenance |
| Benchmark statistics | Manual timing loops | `go test -bench` + `benchstat` | Statistical rigor; `-count=10` gives confidence intervals |
| NMI computation | New NMI impl | Extract existing `nmi()` from leiden_test.go | Already correct, tested |
| Race detection | Manual mutex audit | `go test -race` | Go race detector covers all goroutine interleavings |

**Key insight:** The Go standard library and toolchain cover all performance infrastructure needs. No third-party dependencies are warranted.

## Common Pitfalls

### Pitfall 1: sync.Pool and -benchmem 0 allocs/op
**What goes wrong:** Pool is warmed on first call but benchmark measures first call as part of `b.N`. Alloc count shows > 0.
**Why it happens:** `b.N` starts at 1 on the first iteration; Pool is cold.
**How to avoid:** Run benchmark with `-count=3` or more; only repeated calls (iterations 2+) will show 0 allocs. The requirement says "repeated same-size graph calls" — use `-benchmem` and verify allocs/op converges to 0 across iterations. Alternatively, pre-warm Pool outside `b.ResetTimer()`.
**Warning signs:** `allocs/op` shows `1` or `2` even after warmup.

### Pitfall 2: neighborBuf map growth on larger graphs
**What goes wrong:** neighborBuf map starts small from Pool and must grow during phase1 on a 10K-node graph. Growth triggers allocation even with dirty-list.
**Why it happens:** Go maps allocate new buckets when load factor exceeded.
**How to avoid:** When getting state from Pool, check `cap(neighborBuf)` and pre-grow to `g.NodeCount()` before the first pass. Alternatively, allocate with `make(map[NodeID]float64, g.NodeCount())` on first use.
**Warning signs:** allocs/op > 0 after first warm call.

### Pitfall 3: Race condition in Detect with shared package-level pool
**What goes wrong:** `sync.Pool` itself is goroutine-safe, but state objects must not be aliased. If `Put()` is called before all use of the state is complete, another goroutine's `Get()` returns a state still in use.
**Why it happens:** Forgetting `defer pool.Put(st)` guarantees or using state after Put.
**How to avoid:** Always use `defer pool.Put(st)` immediately after `Get()`. Verify with `go test -race -count=5 ./graph/...`.
**Warning signs:** `-race` reports a write/read conflict in louvain_state fields.

### Pitfall 4: Football/Polbooks NMI threshold selection
**What goes wrong:** Setting NMI threshold too high causes flaky tests (algorithm is stochastic, NMI varies by seed).
**Why it happens:** Community detection algorithms don't always recover ground-truth communities perfectly; Football has overlapping communities.
**How to avoid:** Use conservative thresholds (NMI >= 0.5 for Football, NMI >= 0.6 for Polbooks) and fix seeds. Document expected NMI in test log output.
**Warning signs:** Tests pass on one seed but fail on another.

### Pitfall 5: Louvain edge case gaps vs Leiden
**What goes wrong:** Louvain_test.go is missing some of the 8 required edge cases that Leiden already covers.
**Why it happens:** Phase 03 added edge cases for Leiden; Louvain was written in Phase 02 before the full 8-case spec.
**How to avoid:** Audit `louvain_test.go` against the 8-case list: empty ✓, single ✓, disconnected ✓, giant+singletons ?, 2-node ✓, zero-resolution ?, complete graph ?, self-loop ?. Add missing cases.
**Warning signs:** `go test -v ./graph/...` shows fewer test cases for Louvain than Leiden.

## Code Examples

### sync.Pool integration in louvain_state.go
```go
// Source: https://pkg.go.dev/sync#Pool (Go 1.26 stdlib)
var louvainStatePool = sync.Pool{
    New: func() any {
        return &louvainState{
            partition:     make(map[NodeID]int),
            commStr:       make(map[int]float64),
            neighborBuf:   make(map[NodeID]float64),
            neighborDirty: make([]NodeID, 0, 64),
        }
    },
}

func acquireLouvainState(g *Graph, seed int64) *louvainState {
    st := louvainStatePool.Get().(*louvainState)
    st.reset(g, seed)
    return st
}

func releaseLouvainState(st *louvainState) {
    louvainStatePool.Put(st)
}
```

### Dirty-list reset in phase1
```go
// After computing neighbor weights for node n:
for _, nbr := range g.Neighbors(n) {
    st.neighborBuf[nbr.To] += nbr.Weight
    st.neighborDirty = append(st.neighborDirty, nbr.To)
}
// ... use st.neighborBuf for ΔQ computation ...
// Cleanup — O(degree(n)) not O(|map|):
for _, k := range st.neighborDirty {
    delete(st.neighborBuf, k)
}
st.neighborDirty = st.neighborDirty[:0]
```

### Benchmark with -benchmem and allocation check
```go
// Source: Go testing package docs
func BenchmarkLouvain10K(b *testing.B) {
    g := generateBA10K()      // generated once, outside timer
    det := NewLouvain(LouvainOptions{Seed: 1})
    det.Detect(g)             // warm up pool
    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        det.Detect(g)
    }
}
```

### benchstat baseline
```bash
# Run with -count=10 for statistical significance
go test ./graph/... -bench=BenchmarkLouvain10K -benchmem -count=10 > bench-baseline.txt
benchstat bench-baseline.txt
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| New map per phase1 pass | sync.Pool + dirty-list | This phase | Eliminates heap allocs on hot path |
| `nmi()` only in leiden_test.go | Shared test helper | This phase | Both algorithms use same NMI logic |
| No benchmark functions | `BenchmarkLouvain10K` / `BenchmarkLeiden10K` | This phase | Enables `go test -bench` regression detection |

**Deprecated/outdated:**
- `newLouvainState(g, seed)` as constructor: will be replaced by `acquireLouvainState` + pool; old constructor stays as internal `reset` method.

## Open Questions

1. **Polbooks ground-truth community count**
   - What we know: 105-node political books graph has 3 known communities (liberal/neutral/conservative)
   - What's unclear: NMI threshold to use — algorithm may find more than 3; NMI against 3-class GT may be low
   - Recommendation: Set NMI >= 0.5 as conservative threshold; log actual NMI for tuning

2. **Louvain edge case: complete graph**
   - What we know: Louvain on a complete graph should place all nodes in one community (Q=0 or slightly negative)
   - What's unclear: Current behavior not verified — needs a test
   - Recommendation: Add test with N=5 complete graph, assert no error, log Q

3. **leidenState neighborBuf**
   - What we know: The context specifies `neighborWeightBuf` dirty-list but `leidenState` does not currently have a neighborBuf field
   - What's unclear: Whether Leiden's phase1 (which reuses louvainState inline) already benefits, or needs its own buf
   - Recommendation: Inspect leiden.go phase1 code path; add neighborBuf to leidenState if it has its own local-move loop

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | All | ✓ | 1.26.1 | — |
| `go test -race` | PERF-02, PERF-04 | ✓ | built-in | — |
| `go test -bench` | PERF-01, TEST-05 | ✓ | built-in | — |
| `benchstat` | TEST-05 | ✗ | — | Wave 0 install: `go install golang.org/x/perf/cmd/benchstat@latest` |

**Missing dependencies with no fallback:**
- None — benchstat is installable via `go install`.

**Missing dependencies with fallback:**
- `benchstat`: not installed; Wave 0 must run `go install golang.org/x/perf/cmd/benchstat@latest`.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` package, Go 1.26.1 |
| Config file | none (standard `go test`) |
| Quick run command | `go test ./graph/... -count=1` |
| Full suite command | `go test -race ./graph/... -count=1 && go test ./graph/... -bench=. -benchmem -count=3` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PERF-01 | Louvain10K < 100ms/op | benchmark | `go test ./graph/... -bench=BenchmarkLouvain10K -benchmem -count=3` | ❌ Wave 0 |
| PERF-01 | Leiden10K < 100ms/op | benchmark | `go test ./graph/... -bench=BenchmarkLeiden10K -benchmem -count=3` | ❌ Wave 0 |
| PERF-02 | Concurrent Detect, no race | race detector | `go test -race ./graph/... -count=3` | ❌ Wave 0 (add concurrent test) |
| PERF-03 | 0 allocs/op after warmup | benchmark | `go test ./graph/... -bench=BenchmarkLouvain10K -benchmem -count=3` | ❌ Wave 0 |
| PERF-04 | go test -race passes | race detector | `go test -race ./graph/... -count=1` | ✅ (existing tests run under -race) |
| TEST-01 | Louvain Q>0.35 (already passing) | unit | `go test ./graph/... -run TestLouvainKarateClub` | ✅ `louvain_test.go` |
| TEST-01 | Leiden Q>0.35 (already passing) | unit | `go test ./graph/... -run TestLeidenKarateClubAccuracy` | ✅ `leiden_test.go` |
| TEST-02 | Football NMI validation | unit | `go test ./graph/... -run TestFootball` | ❌ Wave 0 |
| TEST-03 | Polbooks NMI validation | unit | `go test ./graph/... -run TestPolbooks` | ❌ Wave 0 |
| TEST-04 | 8 edge cases Louvain | unit | `go test ./graph/... -run TestLouvain` | partial ✅ — audit needed |
| TEST-04 | 8 edge cases Leiden | unit | `go test ./graph/... -run TestLeiden` | ✅ `leiden_test.go` |
| TEST-05 | benchstat baseline | manual | `benchstat bench-baseline.txt` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./graph/... -count=1`
- **Per wave merge:** `go test -race ./graph/... -count=1 && go test ./graph/... -bench=BenchmarkLouvain10K -benchmem -count=3`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `graph/benchmark_test.go` — covers PERF-01, PERF-03 (BenchmarkLouvain10K, BenchmarkLeiden10K)
- [ ] `graph/accuracy_test.go` — covers TEST-02, TEST-03 (Football, Polbooks NMI tests)
- [ ] `graph/testdata/football.go` — covers TEST-02 fixture
- [ ] `graph/testdata/polbooks.go` — covers TEST-03 fixture
- [ ] `graph/concurrent_test.go` — covers PERF-02 (concurrent Detect race test)
- [ ] benchstat install: `go install golang.org/x/perf/cmd/benchstat@latest` — required for TEST-05
- [ ] Louvain edge case audit + additions in `louvain_test.go` — covers TEST-04 gap cases

## Project Constraints (from CLAUDE.md)

- **No Co-Authored-By trailer** in commit messages
- **GSD workflow** must be used for all non-trivial tasks
- **Subagent review mandatory** after completing code, design, or plan
- **Single `package graph`** — no sub-packages (from Phase 01 decision)
- **`map[NodeID]int` as Partition** — no external type
- **GOEXPERIMENT=arena explicitly out of scope** (see REQUIREMENTS.md Out of Scope)

## Sources

### Primary (HIGH confidence)
- Go stdlib sync.Pool: https://pkg.go.dev/sync#Pool — Pool behavior, GC interaction, goroutine safety
- Go testing.B: https://pkg.go.dev/testing#B — benchmark patterns, ReportAllocs, ResetTimer
- Go race detector: https://go.dev/doc/articles/race_detector — race detection guarantees
- Existing codebase: `graph/louvain_state.go`, `graph/leiden_state.go`, `graph/leiden_test.go` — direct inspection

### Secondary (MEDIUM confidence)
- benchstat: https://pkg.go.dev/golang.org/x/perf/cmd/benchstat — statistical comparison tool, standard Go perf tooling
- Football network dataset: Girvan & Newman (2002) — 115 nodes, 613 edges, 12 ground-truth communities
- Polbooks dataset: Krebs (2004) — 105 nodes, 441 edges, 3 ground-truth communities (L/N/C)

### Tertiary (LOW confidence)
- NMI thresholds for Football/Polbooks: based on typical Louvain/Leiden performance in literature; actual values must be tuned empirically with fixed seeds

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — Go stdlib only; sync.Pool is well-documented
- Architecture: HIGH — based on direct codebase inspection and established Go patterns
- Pitfalls: HIGH — sync.Pool semantics are well-known; NMI threshold risk is LOW confidence
- Benchmark fixture data: MEDIUM — Football/Polbooks edge lists must be sourced from dataset repos

**Research date:** 2026-03-29
**Valid until:** 2026-09-29 (stable — Go stdlib APIs do not change)
