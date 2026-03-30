# Phase 3: 벤치마크 비교 — Python networkx 대비 성능 비교표 작성 (채택 논거) - Research

**Researched:** 2026-03-30
**Domain:** Python benchmark scripting (python-louvain, networkx, timeit) + README table authoring
**Confidence:** HIGH — all numbers verified by live execution on target hardware

## Summary

This phase creates `scripts/compare.py` that benchmarks `python-louvain` (community package)
on identical Barabasi-Albert graphs to the Go benchmarks, then expands the README Performance
section with a Go vs Python comparison table. All timing numbers below were obtained by
**actually running both suites on the target machine** (Apple M4 arm64) — they are not
estimates from training data.

The key finding is that Go is **~15x faster at 1K nodes** and **~75x faster at 10K nodes**
compared to `community.best_partition`. This widening gap at larger graph sizes is a strong
adoption argument and should be highlighted in the README table.

**Primary recommendation:** Use `timeit.repeat(number=1, repeat=5)` in compare.py, report
the minimum of the repeats (standard practice for wall-clock benchmarks), and hard-code
Go baseline numbers from `bench-baseline.txt` rather than re-running Go from within Python.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- Python benchmark script: `scripts/compare.py` committed to repo
- Package: `python-louvain` (import as `community`) — `community.best_partition(G)`
- Graph sizes: 1K and 10K nodes
- Metric: Time + Speedup multiplier (e.g. "75x faster")
- README location: `## Performance` section expanded (not a separate file)
- Hardware note: Apple M4 footer, Python 3.x + networkx x.x + community x.x versions shown
- Install line: `pip install networkx python-louvain` immediately below the table

### Claude's Discretion
- compare.py graph generation method (Barabasi-Albert confirmed best match to Go benchmarks)
- Measurement repetition count (researched — `timeit.repeat(number=1, repeat=5)` recommended)
- README table markdown layout details

### Deferred Ideas (OUT OF SCOPE)
- Separate BENCHMARKS.md file
- Memory comparison
- 100K+ graph sizes
</user_constraints>

---

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| python-louvain | 0.16 (latest) | Louvain community detection in Python | Most widely used Python community detection; wraps networkx |
| networkx | 3.6.1 (already installed) | Graph generation + structure | Required by python-louvain; standard Python graph library |
| timeit | stdlib | Benchmark timing | Built-in; avoids external deps; produces repeatable wall-clock times |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| numpy | 2.4.2 (already installed) | Required by python-louvain | Auto-installed as dependency |

**Installation (for compare.py header comment and README):**
```bash
pip install networkx python-louvain
```

**Environment note:** On macOS with Homebrew Python (externally-managed-environment), users
must use a virtual environment:
```bash
python3 -m venv .venv && source .venv/bin/activate
pip install networkx python-louvain
```

compare.py should include a comment about this. The README install line can stay as the
simple `pip install` form; venv detail belongs in compare.py's header comment only.

**Version verification (live on Apple M4, 2026-03-30):**
- `python-louvain 0.16` — latest on PyPI, confirmed via `pip index versions`
- `networkx 3.6.1` — already installed, confirmed via `pip show networkx`

---

## Architecture Patterns

### Recommended Project Structure
```
scripts/
└── compare.py    # new — Python benchmark + stdout table output
README.md         # expand ## Performance section
```

### Pattern 1: compare.py Structure

**What:** Self-contained benchmark script, no argparse, prints a Markdown table to stdout.
Run once by developer; numbers get copy-pasted (or noted) into README.

**When to use:** Single-purpose scripts that produce human-readable output.

**Recommended structure:**
```python
#!/usr/bin/env python3
"""
Benchmark python-louvain vs loom (Go) community detection.

Install deps:
    pip install networkx python-louvain
    # Or in a venv on macOS:
    #   python3 -m venv .venv && source .venv/bin/activate
    #   pip install networkx python-louvain

Usage:
    python scripts/compare.py
"""
import timeit
import networkx as nx
import community          # python-louvain package

# --- Graph generation (matches Go benchmark: generateBA(n, 5, 42)) ---
def make_ba(n: int) -> nx.Graph:
    return nx.barabasi_albert_graph(n, 5, seed=42)

# --- Timing helper ---
def bench_ms(G: nx.Graph, repeat: int = 5) -> float:
    """Return minimum wall-clock time in ms over `repeat` runs."""
    times = timeit.repeat(
        stmt=lambda: community.best_partition(G),
        number=1,
        repeat=repeat,
    )
    return min(times) * 1000

# --- Main ---
def main():
    import sys
    print(f"Python     : {sys.version.split()[0]}")
    print(f"networkx   : {nx.__version__}")
    print(f"community  : {community.__version__}")
    print()

    sizes = [("1K", 1_000), ("10K", 10_000)]
    # Go baseline from bench-baseline.txt (Apple M4, arm64)
    # go test -bench=. ./graph — medians from bench-baseline.txt
    go_louvain = {"1K": 5.3,  "10K": 50.0}
    go_leiden  = {"1K": 5.7,  "10K": 56.0}

    print("| Graph | Python (community) | Go Louvain | Speedup | Go Leiden | Speedup |")
    print("|-------|-------------------|------------|---------|-----------|---------|")
    for label, n in sizes:
        G = make_ba(n)
        py_ms = bench_ms(G)
        sp_louvain = py_ms / go_louvain[label]
        sp_leiden  = py_ms / go_leiden[label]
        print(f"| {label:5} | {py_ms:>9.0f} ms       | "
              f"{go_louvain[label]:>5.0f} ms   | {sp_louvain:>5.0f}x   | "
              f"{go_leiden[label]:>5.0f} ms  | {sp_leiden:>5.0f}x   |")

if __name__ == "__main__":
    main()
```

**Key design choices:**
- `stmt=lambda:` form avoids string-based `setup` complexity
- `number=1, repeat=5` — each repeat is one full algorithm call; min of 5 is standard for
  non-CPU-bound variance
- Go numbers hard-coded from `bench-baseline.txt` (authoritative, reproducible)
- No argparse — this is a one-shot developer tool

### Pattern 2: README Performance Table Expansion

**What:** Expand the existing `## Performance` table to add Python comparison columns.

**Current README state (lines 141–150):**
```markdown
## Performance

Benchmarks run on standard hardware, undirected graphs with random structure:

| Graph size | Algorithm | Time |
|------------|-----------|------|
| 10K nodes  | Louvain   | ~48ms |
| 10K nodes  | Leiden    | ~57ms |
```

**Expanded form (recommended):**
```markdown
## Performance

Benchmarks on Apple M4, Barabasi-Albert graphs (m=5, seed=42):

| Graph | Go Louvain | Go Leiden | Python Louvain* | Speedup (vs Python) |
|-------|-----------|-----------|-----------------|---------------------|
| 1K nodes  | ~5 ms  | ~6 ms   | ~80 ms          | ~15x                |
| 10K nodes | ~50 ms | ~56 ms  | ~3700 ms        | ~75x                |

\* `community.best_partition` from python-louvain 0.16, networkx 3.6.1, Python 3.12, Apple M4.
Reproduce: `pip install networkx python-louvain && python scripts/compare.py`
```

**Note on speedup column:** A single "Speedup" column comparing to Go Louvain is cleaner
than two separate Speedup columns. Use "vs Python Louvain" since that is the one and only
Python library being compared.

### Anti-Patterns to Avoid
- **String-based timeit setup:** `timeit.timeit("community.best_partition(G)", setup="import community")` — G is not in scope; use `globals=` or `lambda`
- **number > 1 in timeit:** `number=5` measures 5 × algorithm calls per repeat, inflating total time; use `number=1`
- **Using time.time() instead of timeit:** susceptible to GIL/OS scheduling jitter; timeit disables gc by default
- **Re-running Go benchmarks from Python:** unnecessary complexity; bench-baseline.txt has the authoritative numbers

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Barabasi-Albert generation | Custom PA algorithm in Python | `nx.barabasi_albert_graph(n, m, seed=42)` | networkx implementation matches Go's `generateBA` parameters; using same seed=42 ensures graph structure equivalence |
| Timing framework | Manual `time.time()` loop | `timeit.repeat(number=1, repeat=5)` | stdlib; disables GC; standard for micro-benchmarks |
| Community detection | Any custom Python Louvain | `community.best_partition(G)` | Locked decision; most common Python baseline users would compare against |

**Key insight:** The value of compare.py is reproducibility and fair comparison — same graph
generation parameters (n, m=5, seed=42) as Go's `generateBA`. Don't diverge.

---

## Actual Benchmark Numbers (verified 2026-03-30, Apple M4)

These are real measured values, not estimates.

### Go (from bench-baseline.txt)
| Size | Louvain | Leiden |
|------|---------|--------|
| 10K  | ~50 ms  | ~56 ms |

### Go (measured live for 1K, temporary benchmark)
| Size | Louvain | Leiden |
|------|---------|--------|
| 1K   | ~5.3 ms | ~5.7 ms |

### Python — `community.best_partition`, networkx 3.6.1, python-louvain 0.16
| Size | timeit min (5 runs) | timeit mean (5 runs) |
|------|--------------------|--------------------|
| 1K   | ~78 ms             | ~92 ms             |
| 10K  | ~3700 ms           | ~4300 ms           |

### Computed Speedups
| Size | Go Louvain vs Python | Go Leiden vs Python |
|------|---------------------|---------------------|
| 1K   | ~15x                | ~14x                |
| 10K  | ~75x                | ~67x                |

**Observation:** The speedup gap widens substantially at 10K. This is the strongest
adoption argument — Python's O(n²) inner loops in pure Python dominate at scale.

---

## Common Pitfalls

### Pitfall 1: Seed=42 Graph Mismatch Between Go and Python
**What goes wrong:** `nx.barabasi_albert_graph(n, 5, seed=42)` produces a different graph than
Go's `generateBA(n, 5, 42)` because they use different RNG implementations and different
preferential-attachment algorithms. The graphs will have the same n/m/seed parameters but
different edge sets.
**Why it happens:** networkx and the custom Go implementation diverge in how they build the
degree list and sample from it.
**How to avoid:** This is **expected and acceptable**. The comparison is fair because both use
BA graphs of the same size/parameter class — the exact edge set doesn't need to match. Add
a comment in compare.py noting this.
**Warning signs:** Do not try to make edge sets identical; it would require porting the exact
Go RNG.

### Pitfall 2: python-louvain Non-Determinism
**What goes wrong:** `community.best_partition` is non-deterministic by default (randomize=True).
Repeated runs show high variance (observed: 78ms–123ms for 1K across 5 runs).
**Why it happens:** random_state=None means different RNG seed each call.
**How to avoid:** Use `timeit.repeat` and report **minimum** (not mean), which captures the
algorithm's best-case execution time. Alternatively pass `random_state=42` to pin the seed,
but this is not standard practice for benchmarks. The compare.py should report min.
**Warning signs:** Large run-to-run variance in Python numbers is expected; min is the
standard "potential performance" metric.

### Pitfall 3: Externally-Managed Python Environment (macOS)
**What goes wrong:** `pip install python-louvain` fails with "externally-managed-environment"
on Homebrew Python 3.12.
**Why it happens:** PEP 668 — Homebrew marks its Python as externally managed.
**How to avoid:** compare.py header comment should note `python3 -m venv .venv` workflow.
README install line stays simple (`pip install networkx python-louvain`) — this is the
canonical form; venv detail is a local environment concern.
**Warning signs:** Error message mentions "externally managed"; solution is virtualenv.

### Pitfall 4: community Module Not Named `community` on PyPI
**What goes wrong:** `pip install community` installs a different, unrelated package.
**Why it happens:** The PyPI package name is `python-louvain` but the import name is `community`.
**How to avoid:** Always use `pip install python-louvain`; import as `import community`.
Document this discrepancy clearly in compare.py header comment and README install line.

### Pitfall 5: timeit lambda Captures Mutable Graph
**What goes wrong:** If G is modified between repeats (it isn't by best_partition, but worth noting),
results could be inconsistent.
**Why it happens:** Python closures capture by reference.
**How to avoid:** Generate G outside timeit (as shown in the pattern above); best_partition
does not modify G in-place (verified: it only reads graph structure).

---

## Code Examples

### Verified python-louvain API (community 0.16, networkx 3.6.1)

```python
# Source: live verification on Apple M4, 2026-03-30
import community          # pip install python-louvain
import networkx as nx

# Signature: community.best_partition(graph, partition=None, weight='weight',
#            resolution=1.0, randomize=None, random_state=None)
G = nx.barabasi_albert_graph(1000, 5, seed=42)
partition = community.best_partition(G)
# Returns: dict[int, int] — node_id -> community_id
# No deprecation warnings on networkx 3.6.1

# Optional: compute modularity
Q = community.modularity(partition, G)
# Signature: community.modularity(partition, graph, weight='weight')
```

### Timeit Pattern for compare.py

```python
# Source: live verification on Apple M4, 2026-03-30
import timeit

def bench_ms(G, repeat=5):
    times = timeit.repeat(
        stmt=lambda: community.best_partition(G),
        number=1,
        repeat=repeat,
    )
    return min(times) * 1000   # ms, minimum of repeats
```

### README Table Layout (verified renders correctly)

```markdown
| Graph | Go Louvain | Go Leiden | Python Louvain* | Speedup (vs Python) |
|-------|-----------|-----------|-----------------|---------------------|
| 1K nodes  | ~5 ms  | ~6 ms   | ~80 ms          | ~15x                |
| 10K nodes | ~50 ms | ~56 ms  | ~3700 ms        | ~75x                |

\* python-louvain 0.16, networkx 3.6.1, Python 3.12, Apple M4.
Reproduce: `pip install networkx python-louvain && python scripts/compare.py`
```

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `python-louvain` uses `G.nodes_iter()` (networkx 1.x) | Uses `G.nodes()` (networkx 2.x+) | networkx 2.0 (2017) | python-louvain 0.16 is compatible with networkx 3.x — no issues |
| Manual `time.time()` loops | `timeit.repeat()` | Best practice; always | More reliable; GC disabled |

**No deprecated patterns identified** for this phase's scope.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Python 3 | compare.py execution | Yes | 3.12.12 | — |
| pip3 | Package install | Yes | 25.3 | — |
| networkx | Graph generation | Yes (already installed) | 3.6.1 | — |
| python-louvain | Python benchmark | Not installed (needs venv) | 0.16 on PyPI | — |
| numpy | python-louvain dep | Yes (already installed) | 2.4.2 | — |
| Go | Go benchmarks (already done) | Yes (bench-baseline.txt exists) | 1.26.1 | Use bench-baseline.txt values |

**Missing dependencies with no fallback:**
- `python-louvain` — not installed in system Python due to PEP 668 (externally managed).
  The compare.py script must be run in a virtualenv. The planner should include a Wave 0
  task: `python3 -m venv .venv && source .venv/bin/activate && pip install networkx python-louvain`
  as the setup step for running compare.py. This is a one-time local setup, not a repo dependency.

**Missing dependencies with fallback:**
- None.

---

## Open Questions

1. **Should compare.py re-run Go benchmarks, or use hard-coded baseline values?**
   - What we know: bench-baseline.txt has authoritative Apple M4 numbers; re-running Go
     requires `go test` in PATH and adds ~13s to script runtime.
   - What's unclear: Whether users running compare.py will have Go installed.
   - Recommendation: Hard-code Go baseline from bench-baseline.txt with a comment
     "# Source: bench-baseline.txt — run `go test -bench=. ./graph/` to regenerate".
     This makes compare.py usable by Python-only users for verification purposes.

2. **Should 1K Go numbers be added to bench-baseline.txt?**
   - What we know: 1K Go numbers (~5ms Louvain, ~5.7ms Leiden) were measured live but
     not in bench-baseline.txt (which only has 10K runs).
   - What's unclear: Whether the planner wants to regenerate bench-baseline.txt or just
     embed the 1K numbers in compare.py and README.
   - Recommendation: Add the 1K Go benchmark to benchmark_test.go as `BenchmarkLouvain1K`
     and `BenchmarkLeiden1K`, then regenerate bench-baseline.txt. Keeps all baseline
     numbers in one authoritative file.

---

## Sources

### Primary (HIGH confidence)
- Live execution on Apple M4 arm64, 2026-03-30 — all Python and Go timing numbers
- `pip index versions python-louvain` — confirmed version 0.16 is current
- `python3 -c "import community; print(community.__version__)"` — confirmed 0.16 API
- `inspect.signature(community.best_partition)` — confirmed function signature

### Secondary (MEDIUM confidence)
- PyPI package page for python-louvain (fetched via pip metadata) — version history,
  deps confirmed (networkx, numpy)

### Tertiary (LOW confidence)
- None used — all claims verified by direct execution.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — packages installed and tested in venv on target machine
- Architecture (compare.py pattern): HIGH — API verified live, all edge cases tested
- Timing numbers: HIGH — measured directly on Apple M4 (same hardware as bench-baseline.txt)
- Speedup ratios: HIGH — computed from live measurements
- Pitfalls: HIGH — Pitfalls 1-4 verified by reproduction; Pitfall 5 is preventive

**Research date:** 2026-03-30
**Valid until:** 2026-06-30 (python-louvain has been at 0.16 since 2021; networkx 3.x stable)
