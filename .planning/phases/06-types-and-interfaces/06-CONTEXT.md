# Phase 06: Types and Interfaces - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure phase)

<domain>
## Phase Boundary

Define `OverlappingCommunityDetector` interface and its associated types (`OverlappingCommunityResult`, `EgoSplittingOptions`) plus the `NewEgoSplitting` constructor stub. Package must compile and all existing tests must continue to pass.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion
All implementation choices are at Claude's discretion — pure infrastructure phase.

Key requirements from REQUIREMENTS.md:
- EGO-01: `OverlappingCommunityDetector` interface with `Detect(g *Graph) (OverlappingCommunityResult, error)` — distinct from `CommunityDetector`, zero breaking changes
- EGO-02: `OverlappingCommunityResult` with `Communities [][]NodeID` (community-first) and `NodeCommunities map[NodeID][]int` (node-first O(1) lookup)
- EGO-03: `EgoSplittingOptions` with `LocalDetector CommunityDetector`, `GlobalDetector CommunityDetector`, `Resolution float64` — both detectors default to Louvain if nil
- EGO-07: `NewEgoSplitting(opts EgoSplittingOptions)` constructor returning `OverlappingCommunityDetector`

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `CommunityDetector` interface in `graph/detector.go` — new `OverlappingCommunityDetector` mirrors its pattern
- `CommunityResult` struct — `OverlappingCommunityResult` follows the same struct design
- `LouvainOptions` / `LeidenOptions` — `EgoSplittingOptions` follows the same options struct pattern
- `NewLouvain` / `NewLeiden` constructors — `NewEgoSplitting` follows the same `New<Type>` pattern

### Established Patterns
- All types in `package graph`, file named after primary concept (new file: `ego_splitting.go`)
- Exported types PascalCase, unexported struct fields camelCase
- Constructor functions `New<Type>(opts <Type>Options) <Interface>`
- Every exported type and function has a doc comment starting with the identifier name
- Compile-time interface satisfaction check: `var _ Interface = (*impl)(nil)` in `_test.go`

### Integration Points
- `graph/detector.go` — `EgoSplittingOptions.LocalDetector` and `GlobalDetector` reference `CommunityDetector`
- `graph/graph.go` — `OverlappingCommunityResult.Communities [][]NodeID` uses existing `NodeID` type
- `graph/detector_test.go` — new `ego_splitting_test.go` should add compile-time check for the new interface

</code_context>

<specifics>
## Specific Ideas

- New file: `graph/ego_splitting.go` — houses all new types and the `EgoSplittingDetector` stub
- `EgoSplittingDetector.Detect` stub returns `OverlappingCommunityResult{}, nil` (implementation comes in Phase 07-08)
- `NewEgoSplitting` should apply nil-detector defaults (Louvain with zero options) before returning

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
