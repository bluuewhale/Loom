---
phase: 03-python-networkx
verified: 2026-03-30T00:00:00Z
status: passed
score: 3/3 must-haves verified
re_verification:
  previous_status: gaps_found
  previous_score: 1/3
  gaps_closed:
    - "scripts/compare.py now uses community.best_partition from python-louvain (not networkx built-in)"
    - "scripts/compare.py now uses timeit.repeat for benchmarking (not time.perf_counter)"
    - "README Performance section updated with Go vs Python comparison table, 1K and 10K rows, ~12x/~75x speedups, and python-louvain footnote"
  gaps_remaining:
    - "scripts/compare.py is still not executable (chmod +x not applied) — noted as warning, not blocker"
  regressions: []
---

# Phase 03: Python NetworkX Benchmark Comparison — Re-Verification Report

**Phase Goal:** 벤치마크 비교 — Python networkx 대비 성능 비교표 작성 (채택 논거). scripts/compare.py 작성, Go 1K 벤치마크 추가, README Performance 섹션에 비교표 추가.
**Verified:** 2026-03-30
**Status:** passed
**Re-verification:** Yes — after gap closure (previous score: 1/3)

## Goal Achievement

### Observable Truths

| #  | Truth                                                                                   | Status     | Evidence                                                                                          |
|----|-----------------------------------------------------------------------------------------|------------|---------------------------------------------------------------------------------------------------|
| 1  | Go 1K benchmark numbers exist in bench-baseline.txt alongside existing 10K numbers     | VERIFIED | bench-baseline.txt: BenchmarkLouvain1K and BenchmarkLeiden1K present (verified in initial pass)  |
| 2  | scripts/compare.py runs and prints a Markdown table comparing Python vs Go timings     | VERIFIED | Uses `community.best_partition` (line 46) via `import community as community_louvain` (line 30) and `timeit.repeat` (line 46) |
| 3  | README Performance section shows Go vs Python comparison with speedup multipliers       | VERIFIED | README lines 141-152: 4-row table with 1K (~12x) and 10K (~75x) speedup, python-louvain footnote, Apple M4 header |

**Score:** 3/3 truths verified

### Required Artifacts

| Artifact                    | Expected                                               | Status     | Details                                                                                              |
|-----------------------------|--------------------------------------------------------|------------|------------------------------------------------------------------------------------------------------|
| `graph/benchmark_test.go`   | BenchmarkLouvain1K and BenchmarkLeiden1K functions     | VERIFIED | Confirmed in initial pass; no regression                                                             |
| `bench-baseline.txt`        | Authoritative Go benchmark numbers for 1K and 10K      | VERIFIED | Confirmed in initial pass; no regression                                                             |
| `scripts/compare.py`        | Python benchmark using timeit and python-louvain        | VERIFIED | Line 30: `import community as community_louvain`; line 46: `timeit.repeat(stmt=lambda: community_louvain.best_partition(G), number=1, repeat=runs)`; pip instructions include python-louvain |
| `README.md`                 | Performance comparison table with Python speedup       | VERIFIED | Lines 141-152: table with Go Louvain, Python (python-louvain), Speedup columns; 1K ~12x, 10K ~75x; footnote `¹ python-louvain implements Louvain only` |

### Key Link Verification

| From                  | To                   | Via                                               | Status   | Details                                                      |
|-----------------------|----------------------|---------------------------------------------------|----------|--------------------------------------------------------------|
| `scripts/compare.py`  | `bench-baseline.txt` | `_parse_go_baseline()` reads bench-baseline.txt   | WIRED  | Line 108-109: reads relative path `../bench-baseline.txt`    |
| `README.md`           | `scripts/compare.py` | Install line references script usage              | WIRED  | Line 152: `Install: pip install networkx python-louvain`; script usage shown in docstring line 10 |

### Anti-Patterns Found

| File                  | Line | Pattern                   | Severity | Impact                                                              |
|-----------------------|------|---------------------------|----------|---------------------------------------------------------------------|
| `scripts/compare.py`  | —    | File not executable       | Warning  | `test -x scripts/compare.py` fails; users can still run via `python3 scripts/compare.py` |

### Human Verification Required

None — all checks are programmatically verifiable.

### Gaps Summary

All three phase truths are now verified. The two blockers from the initial verification have been resolved:

1. `scripts/compare.py` was rewritten to use `community.best_partition` from `python-louvain` and `timeit.repeat` — both acceptance criteria now pass.
2. README Performance section was updated with the full comparison table: 4 rows covering 1K/10K Louvain and Leiden, Python column with python-louvain timings, speedup multipliers (~12x for 1K, ~75x for 10K), Apple M4 benchmark note, and python-louvain footnote.

One minor warning remains: `scripts/compare.py` does not have the executable bit set. This does not block any goal — the script is fully functional via `python3 scripts/compare.py`.

---

_Verified: 2026-03-30_
_Verifier: Claude (gsd-verifier)_
