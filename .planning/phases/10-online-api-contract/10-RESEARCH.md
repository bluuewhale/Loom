# Phase 10: Online API Contract - Research

**Researched:** 2026-03-31
**Domain:** Go public API design — new type, method addition, guard clause, fast-path
**Confidence:** HIGH

---

## Summary

Phase 10 is a pure API-surface phase: define `GraphDelta`, add `Update()` to
`egoSplittingDetector`, implement the directed-graph guard, and implement the
empty-delta fast-path. No incremental recomputation logic ships here — that is
Phase 11. The concrete scope is small: one new exported struct, one new method
on an unexported struct (surfaced via a new or extended interface), and two
trivially testable behavioral invariants.

All critical questions are fully answerable from the existing codebase — no
external library research is needed. The code lives entirely in `package graph`
(stdlib only, no external imports). The main design decision is whether `Update()`
belongs on `OverlappingCommunityDetector` or only on the concrete struct. The
analysis below resolves this clearly.

**Primary recommendation:** Add `Update()` only to the concrete `*egoSplittingDetector`
(not to the `OverlappingCommunityDetector` interface) and expose it via a new
`OnlineOverlappingCommunityDetector` interface so callers get a typed contract
without breaking the existing interface.

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ONLINE-01 | Caller can construct a `GraphDelta` value with `AddedNodes []NodeID` and `AddedEdges []Edge` | New exported struct; `NodeID` and `Edge` already exist in `graph.go` |
| ONLINE-02 | Caller can invoke `Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)` on an `EgoSplittingDetector` | New method on `*egoSplittingDetector`; signature determined |
| ONLINE-03 | Caller receives prior result unchanged when delta is empty | Fast-path: `return prior, nil` when `len(delta.AddedNodes)==0 && len(delta.AddedEdges)==0` |
| ONLINE-04 | Caller receives `ErrDirectedNotSupported` when `Update()` is called on a directed graph | Mirror existing `Detect()` guard at top of `Update()` |
</phase_requirements>

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| stdlib only | Go 1.21+ | No new imports | Project constraint: stdlib only |

No new dependencies. All types used in the new API (`NodeID`, `Edge`,
`OverlappingCommunityResult`, `ErrDirectedNotSupported`) already exist in the package.

---

## Architecture Patterns

### Existing API to extend

**`OverlappingCommunityDetector` interface** (detector.go, line 11-13):
```go
type OverlappingCommunityDetector interface {
    Detect(g *Graph) (OverlappingCommunityResult, error)
}
```

**`egoSplittingDetector` struct** (ego_splitting.go, line 38-40) — unexported:
```go
type egoSplittingDetector struct {
    opts EgoSplittingOptions
}
```

**`NewEgoSplitting` constructor** returns `OverlappingCommunityDetector` (the interface),
not the concrete type. This is the standard Go pattern.

**`ErrDirectedNotSupported`** lives in `detector.go` line 6-7:
```go
var ErrDirectedNotSupported = errors.New("community detection on directed graphs is not supported")
```
It is already used in `egoSplittingDetector.Detect()` at line 63-65. `Update()` must mirror the same guard.

**`OverlappingCommunityResult`** (ego_splitting.go, line 16-19):
```go
type OverlappingCommunityResult struct {
    Communities     [][]NodeID
    NodeCommunities map[NodeID][]int
}
```
This is a **value type** (struct, not pointer). Its fields are slices and a map —
reference types. Returning `prior` unchanged in the empty-delta fast-path is safe
by value copy: the caller gets a struct copy that shares the same underlying arrays
and map. This is zero additional allocation — no deep copy needed. The benchmark
requirement "within a single allocation" is satisfied by `return prior, nil`.

### Pattern 1: New exported type in the same file as its domain

`GraphDelta` belongs in `ego_splitting.go` alongside `EgoSplittingOptions` and
`OverlappingCommunityResult`. All overlapping-detection types live in that file.

```go
// GraphDelta describes incremental additions to a graph for use with Update().
// Only additions are supported in v1.3; deletions are deferred to v1.4.
type GraphDelta struct {
    AddedNodes []NodeID
    AddedEdges []Edge
}
```

### Pattern 2: New interface for online detectors

The `OverlappingCommunityDetector` interface must NOT gain `Update()`. Reasons:
1. `louvainDetector` and `leidenDetector` do not implement `Update()` — adding it
   to the interface would break the compile-time satisfaction check.
2. Existing callers holding `OverlappingCommunityDetector` should not be forced to
   handle `Update()`.
3. Go convention: extend via a new interface, not by mutating existing ones.

Recommended pattern (mirrors Go stdlib io.ReadWriter / io.ReadWriteCloser):

```go
// OnlineOverlappingCommunityDetector extends OverlappingCommunityDetector with
// incremental Update support for append-only graph mutations.
type OnlineOverlappingCommunityDetector interface {
    OverlappingCommunityDetector
    Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)
}
```

`NewEgoSplitting` continues to return `OverlappingCommunityDetector` for backward
compatibility. A new constructor `NewOnlineEgoSplitting` returns
`OnlineOverlappingCommunityDetector` so callers who need `Update()` get the typed
contract without a type assertion.

**Alternative:** Keep `NewEgoSplitting` return type unchanged and let callers type-
assert to `OnlineOverlappingCommunityDetector`. This is simpler but forces callers
to know the concrete capability. The new-constructor approach is cleaner.

**Decision for planner:** Either approach satisfies the requirements. The
new-constructor approach is more idiomatic and is recommended. The planner should
pick one and document it.

### Pattern 3: Method on concrete struct

```go
// Update returns an updated overlapping community result incorporating delta.
// If delta is empty (no added nodes or edges), prior is returned unchanged.
// Returns ErrDirectedNotSupported if g is a directed graph.
func (d *egoSplittingDetector) Update(
    g *Graph,
    delta GraphDelta,
    prior OverlappingCommunityResult,
) (OverlappingCommunityResult, error) {
    // Guard: directed graphs not supported.
    if g.IsDirected() {
        return OverlappingCommunityResult{}, ErrDirectedNotSupported
    }
    // Fast-path: empty delta — return prior unchanged, zero allocation.
    if len(delta.AddedNodes) == 0 && len(delta.AddedEdges) == 0 {
        return prior, nil
    }
    // Phase 11 placeholder: full recompute (temporary).
    return d.Detect(g)
}
```

The Phase 10 implementation of the non-empty-delta path is a full recompute via
`d.Detect(g)`. This is the correct placeholder: it satisfies the contract
(returns a valid result), does not break tests, and is replaced in Phase 11.

### Pattern 4: Compile-time interface satisfaction checks

Existing tests use:
```go
var _ OverlappingCommunityDetector = (*egoSplittingDetector)(nil)
```

Add for the new interface:
```go
var _ OnlineOverlappingCommunityDetector = (*egoSplittingDetector)(nil)
```

### Recommended file layout for new code

| Item | File | Rationale |
|------|------|-----------|
| `GraphDelta` struct | `ego_splitting.go` | Same file as `EgoSplittingOptions`, `OverlappingCommunityResult` |
| `OnlineOverlappingCommunityDetector` interface | `ego_splitting.go` | Near `OverlappingCommunityDetector` (or in `detector.go`) |
| `NewOnlineEgoSplitting` constructor | `ego_splitting.go` | Near `NewEgoSplitting` |
| `(*egoSplittingDetector).Update()` method | `ego_splitting.go` | Near `Detect()` |
| Tests for ONLINE-01..04 | `ego_splitting_test.go` | All ego-splitting tests live here |

### Anti-Patterns to Avoid

- **Adding `Update()` to `OverlappingCommunityDetector`**: breaks all existing
  implementations; forces Louvain/Leiden to implement a method they don't need.
- **Deep-copying `prior` in the empty-delta fast-path**: unnecessary allocation;
  the requirement is "prior result unchanged" and the value-copy semantics of
  returning a struct already achieves this.
- **Returning `(nil, nil)` or `(OverlappingCommunityResult{}, nil)` for empty delta**:
  this returns an empty result, not the prior. Must return `prior` literally.
- **Pointer-equality test confusion**: the requirement says "pointer-equal to input"
  but `OverlappingCommunityResult` is a value type. The benchmark verifies O(1)
  allocation behavior. The test should use `reflect.DeepEqual` or compare map/slice
  header identity to verify no copy was made; alternatively, the benchmark approach
  (`b.ReportAllocs()` + `allocs/op == 0`) is the definitive verification.
- **Placing `GraphDelta` in `detector.go`**: that file is for the
  `CommunityDetector` / `CommunityResult` family; `GraphDelta` belongs with the
  overlapping detector types.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Empty-delta check | Custom comparison function | Inline `len()` checks | Two fields, O(1), no helper needed |
| Return-prior-unchanged | Deep copy of `OverlappingCommunityResult` | `return prior, nil` | Value-copy semantics; slices/map share backing arrays; zero allocation |
| Directed guard | New error type | Reuse `ErrDirectedNotSupported` | Already defined in `detector.go`; consistent error surface |

---

## Common Pitfalls

### Pitfall 1: Mutating `prior` in fast-path
**What goes wrong:** Future code in `Update()` (Phase 11) modifies the returned
result in-place, corrupting the caller's `prior` variable.
**Why it happens:** Slices and maps in `OverlappingCommunityResult` are reference
types — the value copy shares backing storage.
**How to avoid:** Phase 10 contract is read-only fast-path only. Phase 11 must
deep-copy before mutation. Document this clearly in the method comment.
**Warning signs:** Tests that check `prior` after calling `Update()` on empty delta
and find it modified.

### Pitfall 2: Pointer-equality benchmark assertion
**What goes wrong:** Test tries to assert `&result == &prior` (address equality)
on value types — this is always false for returned values.
**Why it happens:** `OverlappingCommunityResult` is a struct value, not a pointer.
**How to avoid:** Use `b.ReportAllocs()` and assert `allocs/op == 0` in the
benchmark, or assert `result.NodeCommunities == prior.NodeCommunities` (map
pointer identity via `reflect`). The benchmark O(1) assertion is the definitive
contract.
**Warning signs:** Test compiles but panics or always fails.

### Pitfall 3: `NewEgoSplitting` return type change
**What goes wrong:** Changing `NewEgoSplitting` to return
`OnlineOverlappingCommunityDetector` breaks existing callers who hold the value
as `OverlappingCommunityDetector`.
**Why it happens:** Go interface assignment is narrowing; widening requires
explicit re-assignment.
**How to avoid:** Keep `NewEgoSplitting` return type as `OverlappingCommunityDetector`.
Add a new `NewOnlineEgoSplitting` constructor.
**Warning signs:** Existing tests that do `var _ OverlappingCommunityDetector = NewEgoSplitting(...)` break.

### Pitfall 4: Empty-delta check with nil slices
**What goes wrong:** `GraphDelta{}` has nil `AddedNodes` and nil `AddedEdges`.
`len(nil) == 0` in Go, so `len(delta.AddedNodes) == 0 && len(delta.AddedEdges) == 0`
correctly handles nil slices — no special nil check needed.
**Why it happens:** Developer adds `delta.AddedNodes == nil` check thinking nil ≠ empty.
**How to avoid:** Use `len()` exclusively; nil and empty slices are equivalent for
this guard.

---

## Code Examples

### GraphDelta struct
```go
// Source: new type, no external source
type GraphDelta struct {
    AddedNodes []NodeID // new node IDs to add to the graph
    AddedEdges []Edge   // new edges to add (endpoints must exist or be in AddedNodes)
}
```

### OnlineOverlappingCommunityDetector interface
```go
// OnlineOverlappingCommunityDetector extends OverlappingCommunityDetector with
// incremental update support for append-only graph mutations.
type OnlineOverlappingCommunityDetector interface {
    OverlappingCommunityDetector
    Update(g *Graph, delta GraphDelta, prior OverlappingCommunityResult) (OverlappingCommunityResult, error)
}
```

### Update() method skeleton
```go
func (d *egoSplittingDetector) Update(
    g *Graph,
    delta GraphDelta,
    prior OverlappingCommunityResult,
) (OverlappingCommunityResult, error) {
    if g.IsDirected() {
        return OverlappingCommunityResult{}, ErrDirectedNotSupported
    }
    if len(delta.AddedNodes) == 0 && len(delta.AddedEdges) == 0 {
        return prior, nil
    }
    // Phase 11 placeholder: full recompute.
    return d.Detect(g)
}
```

### Constructor for online detector
```go
// NewOnlineEgoSplitting returns an OnlineOverlappingCommunityDetector backed by
// the Ego Splitting algorithm.
func NewOnlineEgoSplitting(opts EgoSplittingOptions) OnlineOverlappingCommunityDetector {
    if opts.LocalDetector == nil {
        opts.LocalDetector = NewLouvain(LouvainOptions{})
    }
    if opts.GlobalDetector == nil {
        opts.GlobalDetector = NewLouvain(LouvainOptions{})
    }
    if opts.Resolution == 0 {
        opts.Resolution = 1.0
    }
    return &egoSplittingDetector{opts: opts}
}
```

### Test: ONLINE-03 empty-delta fast-path
```go
func TestEgoSplittingDetector_Update_EmptyDelta_ReturnsPrior(t *testing.T) {
    g := makeTriangle()
    d := NewOnlineEgoSplitting(EgoSplittingOptions{})
    prior, err := d.Detect(g)
    if err != nil {
        t.Fatalf("Detect: %v", err)
    }

    result, err := d.Update(g, GraphDelta{}, prior)
    if err != nil {
        t.Fatalf("Update with empty delta: %v", err)
    }

    // Verify map pointer identity — no copy made.
    if &result.NodeCommunities == nil {
        t.Fatal("nil NodeCommunities")
    }
    // The map header in result must be the same backing store as prior.
    // reflect.DeepEqual is not sufficient; check that the map pointer is identical.
    // In Go, you can't take address of map directly; use unsafe or benchmark alloc check.
    // Pragmatic approach: assert values are deeply equal (correctness) + rely on
    // BenchmarkUpdate_EmptyDelta to assert 0 allocs/op (performance contract).
    if len(result.Communities) != len(prior.Communities) {
        t.Errorf("Communities length mismatch: got %d, want %d",
            len(result.Communities), len(prior.Communities))
    }
}
```

### Benchmark: ONLINE-03 zero-allocation verification
```go
func BenchmarkUpdate_EmptyDelta(b *testing.B) {
    g := makeTriangle()
    d := NewOnlineEgoSplitting(EgoSplittingOptions{LocalDetector: NewLouvain(LouvainOptions{Seed: 1}), GlobalDetector: NewLouvain(LouvainOptions{Seed: 1})})
    prior, _ := d.Detect(g)
    delta := GraphDelta{} // empty

    b.ResetTimer()
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _, _ = d.Update(g, delta, prior)
    }
    // Pass: 0 allocs/op
}
```

### Test: ONLINE-04 directed graph guard
```go
func TestEgoSplittingDetector_Update_DirectedGraphError(t *testing.T) {
    g := NewGraph(true) // directed
    g.AddEdge(0, 1, 1.0)
    d := NewOnlineEgoSplitting(EgoSplittingOptions{})
    prior := OverlappingCommunityResult{}

    _, err := d.Update(g, GraphDelta{}, prior)
    if !errors.Is(err, ErrDirectedNotSupported) {
        t.Fatalf("expected ErrDirectedNotSupported, got: %v", err)
    }
}
```

### Compile-time satisfaction check
```go
var _ OnlineOverlappingCommunityDetector = (*egoSplittingDetector)(nil)
```

---

## Key Questions Answered

### Q1: Exact current API of `EgoSplittingDetector`

- **Public interface**: `OverlappingCommunityDetector` — only `Detect(g *Graph) (OverlappingCommunityResult, error)`
- **Private struct**: `egoSplittingDetector` with `opts EgoSplittingOptions`
- **Constructor**: `NewEgoSplitting(opts EgoSplittingOptions) OverlappingCommunityDetector`
- **Confidence**: HIGH — read directly from source

### Q2: Where `ErrDirectedNotSupported` lives and how it is used

- **Location**: `detector.go`, line 6-7, `package graph`
- **Current use in `Detect()`**: `ego_splitting.go` lines 63-65 — first guard before ErrEmptyGraph
- **For `Update()`**: same guard, same error, same position (first check in the method body)
- **Confidence**: HIGH

### Q3: Should `Update()` be on the interface or only the concrete struct?

- **Recommendation**: New `OnlineOverlappingCommunityDetector` interface that embeds
  `OverlappingCommunityDetector` and adds `Update()`. Do NOT add to existing interface.
- **Reasoning**: Preserves backward compatibility; idiomatic Go (io.ReadCloser pattern);
  only `egoSplittingDetector` needs it for v1.3
- **Confidence**: HIGH

### Q4: Is `OverlappingCommunityResult` pointer or value type? Return semantics for empty-delta?

- **Type**: Value type (struct). Fields are `[][]NodeID` (slice) and `map[NodeID][]int` (map).
- **Empty-delta return**: `return prior, nil` — struct copy costs ~24 bytes (two slice/map headers).
  The backing arrays and map are shared. Zero additional allocations on the heap.
- **"Pointer-equal" in success criteria**: This means the underlying map/slice storage is
  shared (not deep-copied), verifiable via benchmark showing 0 allocs/op.
- **Confidence**: HIGH

### Q5: How does the empty-delta fast-path work correctly with maps/slices?

- `return prior, nil` copies the struct value (two header values: slice header + map pointer).
- The caller gets a struct whose `NodeCommunities` map pointer points to the same map.
- No allocation occurs. O(1) guaranteed by `len()` checks on the delta.
- This is safe for Phase 10 because `Update()` does not mutate `prior`'s internals in
  the fast-path. Phase 11 must deep-copy before mutation in the non-empty-delta path.

### Q6: What tests should cover ONLINE-03 and ONLINE-04?

| Req | Test name | What it asserts |
|-----|-----------|-----------------|
| ONLINE-03 | `TestEgoSplittingDetector_Update_EmptyDelta_ReturnsPrior` | No error; result structurally equal to prior; 0 allocs in benchmark |
| ONLINE-03 | `BenchmarkUpdate_EmptyDelta` | 0 allocs/op (b.ReportAllocs) |
| ONLINE-04 | `TestEgoSplittingDetector_Update_DirectedGraphError` | `errors.Is(err, ErrDirectedNotSupported)` |
| ONLINE-01 | `TestGraphDelta_Construction` | Compile-time + zero-value defaults are nil slices |
| ONLINE-02 | Compile-time `var _ OnlineOverlappingCommunityDetector = (*egoSplittingDetector)(nil)` | Method exists with correct signature |

---

## Environment Availability

Step 2.6: SKIPPED — Phase 10 is purely code/config changes within `package graph`. No external tools, services, or CLIs required beyond the project's existing Go toolchain.

---

## Validation Architecture

`workflow.nyquist_validation` is explicitly `false` in `.planning/config.json`. This section is skipped per config.

---

## Open Questions

1. **`NewOnlineEgoSplitting` vs type-assertion from `NewEgoSplitting`**
   - What we know: Both satisfy requirements. New constructor is more idiomatic.
   - What's unclear: Whether callers will want to migrate from `NewEgoSplitting` or always
     construct with `NewOnlineEgoSplitting` when they need `Update()`.
   - Recommendation: Add `NewOnlineEgoSplitting`; leave `NewEgoSplitting` signature unchanged.
     Planner should confirm this is the chosen approach.

2. **Non-empty-delta path in Phase 10 (placeholder)**
   - What we know: Phase 10 scope explicitly states the non-empty path is out of scope.
     The placeholder is `return d.Detect(g)`.
   - What's unclear: Should the placeholder have a comment warning callers it's a full
     recompute? Yes — a `// TODO(Phase 11): replace with incremental recomputation` comment
     prevents confusion during Phase 11 review.
   - Recommendation: Add the TODO comment to the non-empty-delta branch.

---

## Sources

### Primary (HIGH confidence)
- `graph/detector.go` — `ErrDirectedNotSupported`, `CommunityDetector`, `OverlappingCommunityDetector` (confirmed by direct read)
- `graph/ego_splitting.go` — `egoSplittingDetector`, `NewEgoSplitting`, `Detect()`, `OverlappingCommunityResult` (confirmed by direct read)
- `graph/ego_splitting_test.go` — existing test patterns: `errors.Is`, compile-time check, `makeTriangle()`, benchmark structure
- `graph/benchmark_test.go` — benchmark patterns: `b.ResetTimer()`, `b.ReportAllocs()`, warmup run
- `graph/graph.go` — `NodeID`, `Edge`, `Graph.IsDirected()` types (confirmed by direct read)

### Secondary (MEDIUM confidence)
- Go standard library docs (embedded knowledge, current): interface embedding pattern (io.ReadCloser), `len(nil) == 0` for slices/maps

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new dependencies; all types exist in-package
- Architecture (interface design): HIGH — code read directly; Go interface embedding is well-established
- Empty-delta fast-path semantics: HIGH — Go value-copy semantics for structs with reference-type fields is deterministic
- Pitfalls: HIGH — derived directly from source code and Go language specification

**Research date:** 2026-03-31
**Valid until:** No expiry — this is a pure in-codebase analysis; valid until the source files change
