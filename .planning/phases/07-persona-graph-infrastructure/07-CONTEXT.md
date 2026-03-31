# Phase 07: Persona Graph Infrastructure - Context

**Gathered:** 2026-03-30
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement and validate Algorithm 1 (ego-net construction) and Algorithm 2 (persona graph generation) as unexported helpers inside `graph/ego_splitting.go`. These two algorithms are validated in isolation on hand-crafted small graphs before being wired into the full Detect pipeline (Phase 08). Phase 07 does NOT wire these into `Detect()` — that stub continues to return `ErrNotImplemented`.

</domain>

<decisions>
## Implementation Decisions

### PersonaID Space & personaMap Design
- PersonaID base: `maxNodeID + 1` — scan `g.Nodes()` for maximum NodeID, add 1. Collision-safe for any NodeID space (not just 0-indexed contiguous).
- Forward persona map: `map[NodeID]map[int]NodeID` — outer key is original NodeID, inner key is local community ID (from the relevant ego-net partition), value is PersonaID.
- Reverse map: `map[NodeID]NodeID` — flat map from PersonaID → original NodeID. O(1) lookup.
- Helper signature: unexported function returning `(personaGraph *Graph, personaOf map[NodeID]map[int]NodeID, inverseMap map[NodeID]NodeID)`.

### Algorithm 2 Edge-Wiring Condition
- For edge (u,v) in original graph: look up community of v in P_u (v's community in G_u's partition = P_u.Partition[v]) → call it c_v; look up community of u in P_v (u's community in G_v's partition = P_v.Partition[u]) → call it c_u. Wire persona(v, c_v) ↔ persona(u, c_u) in the persona graph. Bidirectional per paper Section 2.
- Missing node handling: if u is absent from G_v's partition (or G_v is empty because v has degree 0), skip that edge direction — no persona created for isolated nodes.
- Duplicate edge prevention: use a `seen map[[2]NodeID]struct{}` pattern (same as graph.go Subgraph()) to deduplicate before adding edges to persona graph.

### Test Strategy
- Hand-crafted test graphs: (1) triangle (3 nodes, all-to-all connected), (2) 4-node barbell (two triangles sharing one bridge edge) — small enough to verify persona assignments and edge wiring by hand.
- Weight validation: assert `personaGraph.TotalWeight() == g.TotalWeight()` directly (matches success criterion 3).
- Test placement: white-box tests in `graph/ego_splitting_test.go` (same package, lowercase function access). Keep `buildPersonaGraph` and `mapPersonasToOriginal` unexported.

### Claude's Discretion
- Function naming for unexported helpers (e.g., `buildEgoNet`, `buildPersonaGraph`, `mapPersonasToOriginal`)
- Whether to combine Algorithm 1+2 into a single function or keep separate
- Internal struct wrappers if needed for the return type

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `g.Subgraph(nodeIDs []NodeID) *Graph` — directly implements Algorithm 1: pass `g.Neighbors(v)` to-node IDs (excluding v)
- `g.Neighbors(id NodeID) []Edge` — returns `[]Edge{To NodeID, Weight float64}` for neighbor traversal
- `g.Nodes() []NodeID` — for iterating all nodes and finding maxNodeID
- `g.NodeCount() int` — available but not used for base (using maxNodeID+1 instead)
- `g.TotalWeight() float64` — for persona graph weight validation
- `CommunityResult.Partition map[NodeID]int` — the output of LocalDetector.Detect() used for community lookup
- `NewGraph(directed bool) *Graph` + `g.AddNode(id, weight)` + `g.AddEdge(from, to, weight)` — for building persona graph
- Dedup pattern from `graph.go Subgraph()`: `seen map[[2]NodeID]struct{}` with `lo/hi` ordering

### Established Patterns
- Unexported helpers alongside exported API in same file (`ego_splitting.go`)
- White-box tests in `ego_splitting_test.go` (same package)
- Doc comments on every exported and unexported function
- Error handling: return `(zero, error)` for invalid inputs; `errors.New(...)` sentinel pattern (already have `ErrNotImplemented`)

### Integration Points
- `ego_splitting.go` — all new helpers live here
- `ego_splitting_test.go` — isolation tests for Algorithm 1 and 2
- `graph/ego_splitting.go:Detect()` stub remains untouched — Phase 08 wires everything in

</code_context>

<specifics>
## Specific Ideas

- Algorithm 1 ego-net: `g.Neighbors(v)` returns `[]Edge` — extract `.To` fields into `[]NodeID`, pass to `g.Subgraph()`. v itself is excluded because Subgraph only includes the passed nodeIDs.
- Karate Club verification (success criterion 5): after buildPersonaGraph + mapPersonasToOriginal, at least one original node should appear in multiple community memberships. Can be tested with a simple assertion that len(NodeCommunities[someNode]) > 1 for at least one node.
- The STATE.md blocker ("Algorithm 2 co-membership edge-wiring condition is subtle") is addressed by the bidirectional lookup decision above.

</specifics>

<deferred>
## Deferred Ideas

- Concurrent ego-net construction (goroutine pool) — deferred per REQUIREMENTS.md to after sequential correctness proven
- Phase 08 wiring into Detect() — explicitly out of scope for Phase 07

</deferred>
