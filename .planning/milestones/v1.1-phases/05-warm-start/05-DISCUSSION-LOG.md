# Phase 05: Warm Start — Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-30
**Phase:** 05-warm-start
**Mode:** --auto (all decisions auto-selected)
**Areas discussed:** API Surface, State Injection Point, Node Handling, Leiden Specifics, Benchmarks

---

## API Surface

| Option | Description | Selected |
|--------|-------------|----------|
| Extend `LouvainOptions`/`LeidenOptions` | Add `InitialPartition map[NodeID]int`; nil = cold start | ✓ |
| New `WarmDetector` interface | Separate `DetectWarm(g, prior)` method | |
| New constructor `NewLouvainWarm(opts, prior)` | Returns a warm-start-specific detector | |

**Auto-selected:** Extend options structs
**Rationale:** Zero-value-safe, no breaking change, fits established API pattern. Caller passes `result.Partition` directly.

---

## State Injection Point

| Option | Description | Selected |
|--------|-------------|----------|
| Inject in `reset()` | Modify reset to accept/use initial partition | ✓ |
| Inject before convergence loop | Skip first `reset()` call, pre-populate state | |
| Inject via new `warmReset()` method | Parallel reset path | |

**Auto-selected:** Inject in `reset()`
**Rationale:** Single code path, clean, maintains pool correctness. Warm partition only applies on first reset (iteration 0); subsequent resets are on supergraph which always starts fresh.

---

## New Node Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Singleton community | New nodes get own community; phase1 moves them | ✓ |
| Neighbor's community | Assign to most-connected neighbor's community | |
| Error | Return error if any node missing from prior partition | |

**Auto-selected:** Singleton community
**Rationale:** Simplest, safe, correct. Phase 1 local moves will naturally place new nodes.

---

## Removed Node Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Silently ignore | Only iterate `g.Nodes()`; stale keys never read | ✓ |
| Error | Return error if prior partition has unknown nodes | |

**Auto-selected:** Silently ignore
**Rationale:** Matches "small changes" use case; caller should not need to pre-filter the partition.

---

## Leiden `refinedPartition` Seeding

| Option | Description | Selected |
|--------|-------------|----------|
| Leave nil (cold) | BFS refinement populates on first pass as usual | ✓ |
| Seed from prior | Copy prior partition into refinedPartition | |

**Auto-selected:** Leave nil
**Rationale:** Warm benefit comes from local-move phase; seeding refinedPartition adds complexity with unclear gain.

---

## Benchmarks

| Option | Description | Selected |
|--------|-------------|----------|
| Add warm vs cold benchmark | 10K-node graph, ±1% edge change scenario | ✓ |
| Skip benchmarks | Not needed | |

**Auto-selected:** Add benchmarks
**Rationale:** Primary value proposition of warm start is speed; must be measurable.

---

## Claude's Discretion

- Exact signature change to `reset()` (parameter vs stored field)
- Whether to add `WarmStart bool` field to `CommunityResult`

## Deferred Ideas

- Streaming/event-driven pipeline — future phase
- Directed graph warm start — v2 scope
- Partial warm start (only re-seed near changed nodes) — future optimization
