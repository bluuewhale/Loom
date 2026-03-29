# Milestones

## v1.0 Community Detection (Shipped: 2026-03-29)

**Phases completed:** 4 phases, 5 plans, 7 tasks

**Key accomplishments:**

- CommunityDetector interface with swappable Louvain/Leiden constructors, CommunityResult/options types, and ErrDirectedNotSupported sentinel in graph/detector.go
- Complete Louvain community detection: phase1 deltaQ local moves, buildSupergraph compression, convergence loop, and edge-case guards — Karate Club Q=0.4156 with 4 communities
- One-liner:
- Football (115-node/613-edge) and Polbooks (105-node/441-edge) fixtures added; NMI accuracy suite validates Louvain+Leiden on 3 benchmarks; all 8 Louvain edge cases covered
- 1. [Rule 1 - Bug] Fixed bestSuperPartition pointer sharing under pool reuse

---
