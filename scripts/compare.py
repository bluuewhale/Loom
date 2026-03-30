#!/usr/bin/env python3
"""
compare.py — Benchmark Go loom vs Python NetworkX community detection.

Runs NetworkX Louvain on a 1K-node Barabasi-Albert graph (same parameters as
the Go BenchmarkLouvain1K / BenchmarkLeiden1K benchmarks) and prints a
side-by-side comparison table.

Usage:
    python3 scripts/compare.py

Requirements:
    pip install networkx

The Go numbers are read from bench-baseline.txt (if present) or default to
the recorded Apple M4 values.
"""

import sys
import time
import os
import re
import statistics


def _require_networkx():
    try:
        import networkx as nx
        return nx
    except ImportError:
        print("ERROR: networkx not installed. Run: pip install networkx", file=sys.stderr)
        sys.exit(1)


def _generate_ba_graph(nx, n=1000, m=5, seed=42):
    """Build a BA preferential-attachment graph matching the Go generateBA fixture."""
    return nx.barabasi_albert_graph(n, m, seed=seed)


def _benchmark_louvain(nx, G, runs=5):
    """Return list of wall-clock seconds for louvain_communities on G."""
    from networkx.algorithms.community import louvain_communities
    # warmup
    louvain_communities(G, seed=42)
    times = []
    for _ in range(runs):
        t0 = time.perf_counter()
        louvain_communities(G, seed=42)
        times.append(time.perf_counter() - t0)
    return times


def _parse_go_baseline(path="bench-baseline.txt"):
    """
    Parse bench-baseline.txt and return {benchmark_name: median_ns} dict.

    Recognises lines of the form:
        BenchmarkLouvain1K-10    1093   5437302 ns/op ...
    """
    results = {}
    if not os.path.exists(path):
        return results
    pattern = re.compile(
        r"^(Benchmark\w+)-\d+\s+\d+\s+(\d+)\s+ns/op"
    )
    raw = {}
    with open(path) as f:
        for line in f:
            m = pattern.match(line.strip())
            if m:
                name, ns = m.group(1), int(m.group(2))
                raw.setdefault(name, []).append(ns)
    for name, vals in raw.items():
        results[name] = statistics.median(vals)
    return results


def _format_ms(ns):
    return f"{ns / 1e6:.1f} ms"


def _speedup(python_s, go_ns):
    go_s = go_ns / 1e9
    if go_s == 0:
        return "N/A"
    return f"{python_s / go_s:.0f}x"


def main():
    nx = _require_networkx()

    print("Generating 1K-node Barabasi-Albert graph (n=1000, m=5, seed=42)...")
    G = _generate_ba_graph(nx)
    print(f"  nodes={G.number_of_nodes()}, edges={G.number_of_edges()}\n")

    print("Benchmarking NetworkX Louvain (5 runs)...")
    louvain_times = _benchmark_louvain(nx, G, runs=5)
    louvain_median_s = statistics.median(louvain_times)
    louvain_min_s = min(louvain_times)
    print(f"  median={louvain_median_s*1000:.1f}ms  min={louvain_min_s*1000:.1f}ms\n")

    # Read Go baselines
    baseline_path = os.path.join(os.path.dirname(__file__), "..", "bench-baseline.txt")
    go = _parse_go_baseline(os.path.normpath(baseline_path))

    go_louvain_ns = go.get("BenchmarkLouvain1K", 5437302)   # Apple M4 default
    go_leiden_ns  = go.get("BenchmarkLeiden1K",  5758121)   # Apple M4 default

    # Print comparison table
    sep = "-" * 66
    print(sep)
    print(f"{'Algorithm':<20} {'Python (NetworkX)':<22} {'Go (loom)':<14} {'Speedup'}")
    print(sep)
    print(
        f"{'Louvain 1K':<20} "
        f"{_format_ms(louvain_median_s * 1e9):<22} "
        f"{_format_ms(go_louvain_ns):<14} "
        f"{_speedup(louvain_median_s, go_louvain_ns)}"
    )
    print(
        f"{'Leiden 1K':<20} "
        f"{'N/A (not in nx)':<22} "
        f"{_format_ms(go_leiden_ns):<14} "
        f"{'—'}"
    )
    print(sep)
    print("\nNote: Python Leiden requires a separate backend (e.g. leidenalg).")
    print("Go numbers sourced from bench-baseline.txt (Apple M4, arm64).")


if __name__ == "__main__":
    main()
