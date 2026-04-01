# Architecture

**Analysis Date:** 2026-04-01

## Pattern Overview

**Overall:** Single-package algorithm library — all production code lives in `graph/` as `package graph`. No layering, no sub-packages, no external dependencies (stdlib only). The design philosophy is a composable interface-first approach where callers select algorithms via `CommunityDetector` or `OverlappingCommunityDetector` interfaces and never touch concrete types directly.

**Key Characteristics:**
- All types, algorithms, state, and utilities in one flat package (`graph/`)
- Interface abstraction (`CommunityDetector`, `OverlappingCommunityDetector`, `OnlineOverlappingCommunityDetector`) decouples algorithm choice from usage
- Stateless detectors — all mutable state is allocated per-`Detect` call and pooled via `sync.Pool`
- No external dependencies — `go.mod` declares `go 1.26` with zero `require` entries
- Determinism controlled by `Seed` option — `Seed != 0` is reproducible, `Seed == 0` is random

## Layers

**Graph Data Layer:**
- Purpose: Core graph representation and traversal primitives
- Location: `graph/graph.go`
- Contains: `Graph` struct, `NodeID`, `Edge`, all graph mutation and query methods
- Depends on: nothing
- Used by: every other file in the package

**Identity / Mapping Layer:**
- Purpose: Maps human-readable string names to integer `NodeID`s
- Location: `graph/registry.go`
- Contains: `NodeRegistry` (forward `map[string]NodeID` + reverse `[]string`)
- Depends on: `graph.go`
- Used by: callers building graphs from string data; not used internally by algorithms

**Algorithm Interface Layer:**
- Purpose: Defines swappable detector contracts and result types
- Location: `graph/detector.go`
- Contains: `CommunityDetector`, `CommunityResult`, `LouvainOptions`, `LeidenOptions`, `NewLouvain`, `NewLeiden`
- Depends on: `graph.go`
- Used by: `louvain.go`, `leiden.go`, `ego_splitting.go`, all callers

**Louvain Implementation:**
- Purpose: Phase-1/Phase-2 modularity-maximizing community detection
- Location: `graph/louvain.go`, `graph/louvain_state.go`
- Contains: `louvainDetector.Detect`, `phase1`, `buildSupergraph`, `normalizePartition`, `reconstructPartition`, `louvainState`, `louvainStatePool`
- Depends on: `graph.go`, `modularity.go`
- Used by: direct callers; also called from `leiden.go` (reuses `phase1`) and `ego_splitting.go` (default local/global detector)

**Leiden Implementation:**
- Purpose: Louvain + BFS refinement guaranteeing connected communities
- Location: `graph/leiden.go`, `graph/leiden_state.go`
- Contains: `leidenDetector.Detect`, `leidenDetector.runOnce`, `refinePartition`, `leidenState`, `leidenStatePool`
- Depends on: `graph.go`, `louvain.go` (reuses `phase1`, `buildSupergraph`, `reconstructPartition`, `normalizePartition`), `modularity.go`
- Used by: direct callers; also usable as local/global detector in `ego_splitting.go`

**Overlapping Community Layer (Ego-Splitting):**
- Purpose: Overlapping community detection via persona-graph construction
- Location: `graph/ego_splitting.go`
- Contains: `egoSplittingDetector`, `OverlappingCommunityDetector`, `OnlineOverlappingCommunityDetector`, `buildPersonaGraph`, `buildPersonaGraphIncremental`, `computeAffected`, `runParallelEgoNets`
- Depends on: `graph.go`, `detector.go` (uses `CommunityDetector` internally for both local ego-net and global persona detection)
- Used by: top-level callers who need overlapping communities

**Quality Metrics:**
- Purpose: Modularity Q and Omega index computation
- Location: `graph/modularity.go`, `graph/omega.go`
- Contains: `ComputeModularity`, `ComputeModularityWeighted`, `OmegaIndex`
- Depends on: `graph.go`
- Used by: `louvain.go`, `leiden.go` (Q tracking per pass); test/accuracy code (Omega)

## Data Flow

**Louvain/Leiden Cold-Start Detection:**

1. Caller constructs `*Graph` via `NewGraph` + `AddEdge`
2. Caller calls `detector.Detect(g)` — enters `louvainDetector.Detect` or `leidenDetector.Detect`
3. Guard checks: directed graph → error; empty → early return
4. State acquired from `sync.Pool` via `acquireLouvainState` / `acquireLeidenState`; `reset()` populates singleton partition + `commStr` cache in ascending `NodeID` order
5. **Phase 1** (`phase1`): nodes sorted then shuffled (seed-controlled); each node evaluated against all neighbor communities; `neighborBuf` + dirty-list pattern avoids full map-clear per node; `commStr` cache updated in-place after each move
6. **Phase 2** (`buildSupergraph`): communities collapsed into supernodes; inter-community edges accumulated via canonical `(min,max)` key to avoid double-counting; new `*Graph` returned
7. `nodeMapping` (original NodeID → current supernode) updated; loop repeats until `moves == 0` or `maxPasses` hit
8. Best-Q partition tracked per pass; final result uses best found via `reconstructPartition` → `normalizePartition`
9. State returned to pool via `defer releaseLouvainState`
10. `CommunityResult{Partition, Modularity, Passes, Moves}` returned to caller

**Leiden Additional Step (between Phase 1 and Phase 2):**
- `refinePartition` runs BFS within each community to split disconnected components
- Aggregation uses `refinedPartition` (not raw `phase1` partition) — this is the key Leiden invariant that guarantees internally-connected communities

**Ego-Splitting Full Detection:**

1. Caller calls `egoSplittingDetector.Detect(g)`
2. `buildPersonaGraph(g, localDetector)`:
   - For each node `v`: `buildEgoNet(g, v)` → `g.Subgraph(neighbors(v))` (excludes `v` itself)
   - Isolated nodes handled inline (no goroutine); non-isolated dispatched to `runParallelEgoNets` (bounded `runtime.GOMAXPROCS(0)` worker pool, each with `cloneDetector(localDet)`)
   - Each worker runs `localDetector.Detect(egoNet)` → partition of `v`'s neighbors
   - Persona nodes allocated (one per community in ego-net); `personaOf`, `inverseMap`, `partitions` maps built
   - Edges rewired: edge `(u,v)` → `(persona_u_for_v's_community, persona_v_for_u's_community)`
3. `globalDetector.Detect(personaGraph)` → flat partition of persona nodes
4. `mapPersonasToOriginal` maps persona partition → `map[NodeID][]int` of overlapping original-node communities
5. Deduplication + compaction of community index space
6. `OverlappingCommunityResult` returned with unexported carry-forward fields (`personaOf`, `inverseMap`, `partitions`, `personaPartition`, `personaGraph`) for `Update()`

**Online Incremental Update (Ego-Splitting):**

1. Caller calls `detector.Update(g, delta, prior)` — `g` is already mutated with the delta applied
2. Empty delta → `prior` returned immediately (zero allocations)
3. Missing carry-forward fields in `prior` → fallback to full `Detect(g)`
4. `computeAffected(g, delta)`: affected = new nodes + all neighbors of edge endpoints (2-hop boundary)
5. **Fast-path** (all affected are newly-added isolated nodes): clone prior persona graph, add isolated persona nodes, skip global detection entirely — `O(|affected|)` cost
6. **General path**: `buildPersonaGraphIncremental` — deep-copy prior maps, delete stale affected personas, recompute ego-nets for affected only, clone prior persona graph, `RemoveEdgesFor(affectedPersonas)`, re-wire only edges incident to affected nodes
7. `warmPartition` built from `prior.personaPartition` (stale persona IDs excluded)
8. Global detection warm-started via `warmStartedDetector(globalDetector, warmPartition)` (sets `InitialPartition`)
9. Same persona→original mapping + compaction as full `Detect`; new `OverlappingCommunityResult` with updated carry-forward fields returned

**State Management:**
- No global mutable state beyond `sync.Pool` entries (`louvainStatePool`, `leidenStatePool`)
- All per-run state (`partition`, `commStr`, `neighborBuf`, `rng`) is scoped to `louvainState` / `leidenState` structs and pool-recycled
- `OverlappingCommunityResult` carries hidden fields for incremental reuse; outer maps are deep-copied, inner maps are shallow-shared for unaffected nodes

## Key Abstractions

**CommunityDetector:**
- Purpose: Swappable algorithm interface for disjoint community detection
- Examples: `graph/detector.go` (interface + constructors), `graph/louvain.go` (Louvain impl), `graph/leiden.go` (Leiden impl)
- Pattern: Strategy — caller selects algorithm at construction time; detection is stateless from caller's perspective

**OverlappingCommunityDetector / OnlineOverlappingCommunityDetector:**
- Purpose: Overlapping detection + incremental update interface
- Examples: `graph/ego_splitting.go`
- Pattern: Extended Strategy — `Update()` method enables online streaming graph updates with incremental recomputation

**louvainState / leidenState (pooled):**
- Purpose: Reusable per-run scratch space to reduce GC pressure
- Examples: `graph/louvain_state.go`, `graph/leiden_state.go`
- Pattern: Object Pool (`sync.Pool`) + dirty-list (`neighborDirty []NodeID`) for selective map cleanup without full `clear()`

**NodeRegistry:**
- Purpose: Bidirectional string↔NodeID mapping for API ergonomics
- Examples: `graph/registry.go`
- Pattern: Dual-index (hash map forward O(1), slice reverse O(1) by index)

## Entry Points

**Louvain Detector:**
- Location: `graph/detector.go` (`NewLouvain`), `graph/louvain.go` (`Detect`)
- Triggers: `detector.Detect(g *Graph)`
- Responsibilities: Disjoint community detection; returns `CommunityResult` with partition + modularity Q + pass/move counts

**Leiden Detector:**
- Location: `graph/detector.go` (`NewLeiden`), `graph/leiden.go` (`Detect`, `runOnce`)
- Triggers: `detector.Detect(g *Graph)`
- Responsibilities: Leiden community detection with BFS refinement; multi-run (`NumRuns`, default 3) when `Seed==0`

**Ego-Splitting Detector:**
- Location: `graph/ego_splitting.go` (`NewEgoSplitting`, `NewOnlineEgoSplitting`)
- Triggers: `detector.Detect(g)` or `detector.Update(g, delta, prior)`
- Responsibilities: Overlapping community detection; online incremental updates for append-only graph mutations

## Error Handling

**Strategy:** Return `(Result, error)` at all public API boundaries; internal functions return errors up the call chain without wrapping.

**Patterns:**
- `ErrDirectedNotSupported` — sentinel in `graph/detector.go`, returned by all `Detect`/`Update` methods when `g.IsDirected()`
- `ErrEmptyGraph` — sentinel in `graph/ego_splitting.go`, returned by `egoSplittingDetector.Detect` on empty graph
- Leiden multi-run: partial failures swallowed if at least one run succeeds; `lastErr` discarded when `bestQ > -Inf`
- Ego-net errors from `localDetector.Detect` bubble up from `buildPersonaGraph` / `buildPersonaGraphIncremental` and abort the whole detection

## Cross-Cutting Concerns

**Logging:** None — library produces no output; all observability is through return values.

**Validation:** Guard clauses at top of each `Detect` function (directed, empty, zero-weight special cases). No defensive copies of input `*Graph` — algorithms treat it as read-only.

**Determinism:** Controlled by `Seed` option. `Seed != 0` → `rand.NewSource(seed)` → fully reproducible. `Seed == 0` → `rand.NewSource(time.Now().UnixNano())` → random. Nodes sorted before shuffle so seed is the sole non-determinism source.

**Concurrency:** Ego-splitting uses `runtime.GOMAXPROCS(0)` worker goroutines for ego-net phase. Each worker gets its own detector clone via `cloneDetector`. Algorithms are safe for concurrent calls on distinct `*Graph` instances; NOT safe to share a single detector instance across goroutines concurrently.

## Architectural Bottlenecks and Optimization Points

### 1. Graph Storage: `map[NodeID][]Edge` Adjacency — HIGH IMPACT
All graph traversal — `Neighbors`, `Strength`, `WeightToComm` — goes through `map[NodeID][]Edge` in `graph/graph.go`. Hash map lookups have poor cache locality vs. CSR (Compressed Sparse Row) arrays. The `phase1` inner loop calls `g.Neighbors(n)` for every node on every pass. For 10K-node graphs this is ~50K map lookups per pass × multiple passes. A CSR representation (sorted `[]NodeID` offsets + flat `[]Edge` array) would improve cache performance significantly at the cost of a mutable-to-immutable transition.

### 2. `phase1` Allocates `g.Nodes()` Slice Every Pass — MEDIUM IMPACT
`phase1` in `graph/louvain.go:172` calls `g.Nodes()` which allocates a new `[]NodeID` slice. This happens every pass in the outer loop (typically 3–10 passes). The `louvainState` pool already exists; the node slice should be added to `louvainState` as a pre-allocated field and refilled via `reset()`.

### 3. `ComputeModularityWeighted` Called Per Supergraph Pass — MEDIUM IMPACT
Both `louvainDetector.Detect` (`louvain.go:96`) and `leidenDetector.runOnce` (`leiden.go:159`) call `ComputeModularityWeighted(g, candidatePartition, resolution)` on the *original* full graph once per outer loop iteration for best-Q tracking. This is O(|V| + |E|) on the original graph at each pass. For a 10K graph with 5 passes this is 5 full graph scans solely for Q tracking. Q can be maintained incrementally using the `commStr` cache already present in `louvainState`.

### 4. `deltaQ` Function and `CommStrength` Method Are Dead Code — LOW IMPACT / CLARITY
`deltaQ` at `graph/louvain.go:263` calls `g.WeightToComm` (O(degree)) and `g.Strength` (O(degree)) — it does NOT use the `neighborBuf` optimization. The actual hot path in `phase1` uses inlined `neighborBuf` accumulation instead. `CommStrength` at `graph/graph.go:245` iterates the entire partition map (O(|V|)). Neither is called from any production code path. Both are dead code that mislead readers about the actual hot path.

### 5. `refinePartition` Allocates per-Community Maps — MEDIUM IMPACT (Leiden only)
`refinePartition` in `graph/leiden.go:224` allocates a fresh `inComm map[NodeID]struct{}` and `visited map[NodeID]bool` per community per iteration. For graphs with many small communities (high resolution setting), this generates many short-lived allocations. Both maps can be pooled or replaced with a flat boolean slice indexed by NodeID (since NodeIDs are dense integers).

### 6. `buildSupergraph` Processes All Directed Adjacency Entries — LOW-MEDIUM IMPACT
`buildSupergraph` in `graph/louvain.go:276` iterates all adjacency entries (both directions for undirected graphs) to accumulate inter-community edge weights, then divides by 2. Applying the canonical `(min,max)` dedup key during iteration (as done in `Subgraph`) would halve the inner loop work and eliminate the division step.

### 7. Ego-Splitting `Detect`/`Update` Code Duplication — MAINTAINABILITY
Steps 4b–5 (deduplication + `Communities`/`NodeCommunities` assembly) are copy-pasted verbatim between `egoSplittingDetector.Detect` (`ego_splitting.go:143–201`) and `egoSplittingDetector.Update` (`ego_splitting.go:278–320`). Any bug fix or change must be applied in two places. Should be extracted into a shared `assembleOverlappingResult(nodeCommunities map[NodeID][]int) OverlappingCommunityResult` helper.

### 8. `newLouvainState` / `newLeidenState` Are Unreachable Production Code — LOW
`newLouvainState` (`louvain_state.go:120`) and `newLeidenState` (`leiden_state.go:127`) are retained with a "backward-compatibility" comment but are not called from any production code path. The pool-based `acquireLouvainState` / `acquireLeidenState` replaced them. Their presence creates two initialization paths and confusion about which is canonical.

---

*Architecture analysis: 2026-04-01*
