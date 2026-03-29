---
phase: 02-interface-louvain-core
plan: 01
subsystem: graph
tags: [go, community-detection, interface, louvain, leiden, modularity]

requires:
  - phase: 01-graph-foundation
    provides: NodeID, *Graph types, NewGraph, WeightToComm, CommStrength

provides:
  - CommunityDetector interface with Detect(g *Graph) (CommunityResult, error)
  - CommunityResult struct (Partition, Modularity, Passes, Moves)
  - LouvainOptions and LeidenOptions with zero-value default semantics
  - NewLouvain constructor returning louvainDetector
  - NewLeiden constructor returning leidenDetector stub (returns error)
  - ErrDirectedNotSupported sentinel error
  - louvain.go stub satisfying interface compile-time check

affects:
  - 02-02 (Louvain full implementation uses louvainDetector.Detect in louvain.go)
  - 03-leiden (LeidenOptions and leidenDetector stub replaced by full implementation)

tech-stack:
  added: []
  patterns:
    - "Constructor returns interface type (NewLouvain returns CommunityDetector not *louvainDetector)"
    - "Unexported concrete struct, exported interface — swappable algorithm pattern"
    - "Stub Detect method in louvain.go allows interface satisfaction before full implementation"
    - "Zero-value option semantics documented in godoc with explicit defaults"

key-files:
  created:
    - graph/detector.go
    - graph/detector_test.go
    - graph/louvain.go
  modified: []

key-decisions:
  - "louvainDetector.Detect lives in louvain.go (plan 02), not detector.go — separation of interface from algorithm"
  - "louvain.go created as minimal stub (panics) to satisfy compile-time interface checks in tests"
  - "LeidenOptions.MaxIterations (not MaxPasses) — distinct naming to differentiate from Louvain semantics"

patterns-established:
  - "Pattern 1: Algorithm constructors return CommunityDetector interface, not concrete pointer"
  - "Pattern 2: Stub implementations panic with informative message; error-returning stubs use errors.New"

requirements-completed: [IFACE-01, IFACE-02, IFACE-03, IFACE-04, IFACE-05, IFACE-06]

duration: 8min
completed: 2026-03-29
---

# Phase 02 Plan 01: Interface + Louvain Core Summary

**CommunityDetector interface with swappable Louvain/Leiden constructors, CommunityResult/options types, and ErrDirectedNotSupported sentinel in graph/detector.go**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-03-29T~10:40:00Z
- **Completed:** 2026-03-29T~10:48:00Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 3

## Accomplishments

- Defined `CommunityDetector` interface — the swappable contract for all community detection algorithms
- Exported all 6 IFACE-* types: CommunityDetector, CommunityResult, LouvainOptions, LeidenOptions, NewLouvain, NewLeiden, ErrDirectedNotSupported
- Leiden stub correctly returns "not yet implemented" error; Louvain Detect deferred to plan 02

## Task Commits

1. **Test (RED): detector interface tests** - `4d411fb` (test)
2. **Implementation (GREEN): detector.go + louvain.go stub** - `7d4cc0c` (feat)

## Files Created/Modified

- `graph/detector.go` - CommunityDetector interface, CommunityResult, LouvainOptions, LeidenOptions, constructors, sentinel error
- `graph/detector_test.go` - 5 behavior tests + compile-time interface satisfaction checks
- `graph/louvain.go` - Minimal stub satisfying CommunityDetector interface (panics); full implementation in plan 02

## Decisions Made

- Created `graph/louvain.go` stub with panicking `Detect` to allow compile-time interface check `var _ CommunityDetector = (*louvainDetector)(nil)` in tests — without this stub, tests would not compile
- `LeidenOptions.MaxIterations` field name (not MaxPasses) to semantically distinguish Leiden from Louvain iteration concepts

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created louvain.go stub to satisfy compile-time interface check**
- **Found during:** Task 1 (GREEN phase)
- **Issue:** Plan required compile-time check `var _ CommunityDetector = (*louvainDetector)(nil)` in test file, but `louvainDetector.Detect` was explicitly excluded from `detector.go`. Without the method, the package would not compile.
- **Fix:** Created `graph/louvain.go` with a panicking stub `Detect` method. This satisfies the interface without implementing algorithm logic, maintaining the plan's intent that algorithm code belongs in `louvain.go`.
- **Files modified:** graph/louvain.go (new)
- **Verification:** `go build ./graph/...` and all 5 tests pass
- **Committed in:** 7d4cc0c (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (blocking — needed for compile)
**Impact on plan:** Necessary for correctness. The stub `louvain.go` is exactly what plan 02 will replace with the full implementation. No scope creep.

## Issues Encountered

None — plan executed cleanly once the compile-time check blocker was resolved via Rule 3.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `graph/detector.go` exports all required types; plan 02 can immediately implement `louvainDetector.Detect` in `louvain.go`
- Compile-time interface checks guarantee any future algorithm implementation satisfies the contract at build time
- `graph/louvain.go` stub is the starting point for plan 02 full Louvain implementation

---
*Phase: 02-interface-louvain-core*
*Completed: 2026-03-29*
