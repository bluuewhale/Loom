# Phase 2 — Plan 01: SUMMARY

## What was done
Replaced `refinePartition` with `refinePartitionInPlace` to eliminate all per-community
heap allocations in the Leiden BFS refinement step.

## Key decisions
- Used sorted `[]commNodePair` + CSR-indexed `[]bool` slices instead of per-community maps
- Sort is `(comm, node)` for deterministic BFS start ordering
- Scratch slices grown lazily (first use per pool lifetime) — no allocs on pool reuse
- Writes directly into `state.refinedPartition` (reuses pooled map via `clear()`)

## Outcome
Leiden 10K: 58,220 → 45,938 allocs/op (−21%), now at Louvain parity.
All 13+ tests pass. No public API changes.
