#!/usr/bin/env python3
"""
compare.py — Benchmark Go loom vs Python community detection libraries.

Runs python-louvain (community.best_partition) and leidenalg (find_partition)
on Barabasi-Albert graphs matching the Go BenchmarkLouvain1K / BenchmarkLouvain10K
benchmarks and prints a side-by-side comparison table.

Usage:
    python3 scripts/compare.py

Requirements:
    pip install networkx python-louvain leidenalg
"""

import sys
import os
import re
import statistics
import timeit


def _require_deps():
    try:
        import networkx as nx
    except ImportError:
        print("ERROR: networkx not installed. Run: pip install networkx python-louvain leidenalg", file=sys.stderr)
        sys.exit(1)
    try:
        import community as community_louvain
    except ImportError:
        print("ERROR: python-louvain not installed. Run: pip install networkx python-louvain leidenalg", file=sys.stderr)
        sys.exit(1)
    try:
        import igraph as ig
        import leidenalg
    except ImportError:
        ig, leidenalg = None, None
    return nx, community_louvain, ig, leidenalg


def _generate_ba_graph(nx, n=1000, m=5, seed=42):
    """Build a BA preferential-attachment graph matching the Go generateBA fixture."""
    return nx.barabasi_albert_graph(n, m, seed=seed)


def _benchmark_louvain(community_louvain, G, runs=5):
    """Return list of wall-clock seconds for community.best_partition on G."""
    community_louvain.best_partition(G, random_state=42)
    times = timeit.repeat(stmt=lambda: community_louvain.best_partition(G, random_state=42), number=1, repeat=runs)
    return times


def _benchmark_leidenalg(ig, leidenalg, G_nx, runs=5):
    """Return list of wall-clock seconds for leidenalg.find_partition on G."""
    ig_G = ig.Graph.from_networkx(G_nx)
    # warmup
    leidenalg.find_partition(ig_G, leidenalg.ModularityVertexPartition, seed=42)
    times = timeit.repeat(
        stmt=lambda: leidenalg.find_partition(ig_G, leidenalg.ModularityVertexPartition, seed=42),
        number=1,
        repeat=runs,
    )
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
    return f"~{python_s / go_s:.0f}x"


def main():
    nx, community_louvain, ig, leidenalg = _require_deps()

    print("Generating 1K-node Barabasi-Albert graph (n=1000, m=5, seed=42)...")
    G1k = _generate_ba_graph(nx, n=1000)
    print(f"  nodes={G1k.number_of_nodes()}, edges={G1k.number_of_edges()}\n")

    print("Benchmarking python-louvain best_partition — 1K nodes (5 runs)...")
    times_1k = _benchmark_louvain(community_louvain, G1k, runs=5)
    louvain_1k_s = min(times_1k)
    print(f"  min={louvain_1k_s*1000:.1f}ms  median={statistics.median(times_1k)*1000:.1f}ms\n")

    if leidenalg:
        print("Benchmarking leidenalg find_partition — 1K nodes (5 runs)...")
        leiden_times_1k = _benchmark_leidenalg(ig, leidenalg, G1k, runs=5)
        leiden_1k_s = min(leiden_times_1k)
        print(f"  min={leiden_1k_s*1000:.1f}ms  median={statistics.median(leiden_times_1k)*1000:.1f}ms\n")
    else:
        leiden_1k_s = None
        print("leidenalg not installed — skipping. Run: pip install leidenalg\n")

    print("Generating 10K-node Barabasi-Albert graph (n=10000, m=5, seed=42)...")
    G10k = _generate_ba_graph(nx, n=10000)
    print(f"  nodes={G10k.number_of_nodes()}, edges={G10k.number_of_edges()}\n")

    print("Benchmarking python-louvain best_partition — 10K nodes (3 runs, slower)...")
    times_10k = _benchmark_louvain(community_louvain, G10k, runs=3)
    louvain_10k_s = min(times_10k)
    print(f"  min={louvain_10k_s*1000:.0f}ms  median={statistics.median(times_10k)*1000:.0f}ms\n")

    if leidenalg:
        print("Benchmarking leidenalg find_partition — 10K nodes (3 runs)...")
        leiden_times_10k = _benchmark_leidenalg(ig, leidenalg, G10k, runs=3)
        leiden_10k_s = min(leiden_times_10k)
        print(f"  min={leiden_10k_s*1000:.0f}ms  median={statistics.median(leiden_times_10k)*1000:.0f}ms\n")
    else:
        leiden_10k_s = None

    # Read Go baselines
    baseline_path = os.path.join(os.path.dirname(__file__), "..", "bench-baseline.txt")
    go = _parse_go_baseline(os.path.normpath(baseline_path))

    go_louvain_1k_ns  = go.get("BenchmarkLouvain1K",  5437302)
    go_leiden_1k_ns   = go.get("BenchmarkLeiden1K",   5758121)
    go_louvain_10k_ns = go.get("BenchmarkLouvain10K", 50000000)
    go_leiden_10k_ns  = go.get("BenchmarkLeiden10K",  57000000)

    sep = "-" * 80
    print(sep)
    print(f"{'Graph size':<14} {'Algorithm':<12} {'Go (loom)':<14} {'Python library':<22} {'Python time':<14} {'Speedup'}")
    print(sep)
    print(f"{'1K nodes':<14} {'Louvain':<12} {_format_ms(go_louvain_1k_ns):<14} {'python-louvain':<22} {louvain_1k_s*1000:.1f} ms{'':<6} {_speedup(louvain_1k_s, go_louvain_1k_ns)}")
    if leiden_1k_s is not None:
        print(f"{'1K nodes':<14} {'Leiden':<12} {_format_ms(go_leiden_1k_ns):<14} {'leidenalg':<22} {leiden_1k_s*1000:.1f} ms{'':<6} {_speedup(leiden_1k_s, go_leiden_1k_ns)}")
    print(f"{'10K nodes':<14} {'Louvain':<12} {_format_ms(go_louvain_10k_ns):<14} {'python-louvain':<22} {louvain_10k_s*1000:.0f} ms{'':<6} {_speedup(louvain_10k_s, go_louvain_10k_ns)}")
    if leiden_10k_s is not None:
        print(f"{'10K nodes':<14} {'Leiden':<12} {_format_ms(go_leiden_10k_ns):<14} {'leidenalg':<22} {leiden_10k_s*1000:.0f} ms{'':<6} {_speedup(leiden_10k_s, go_leiden_10k_ns)}")
    print(sep)
    print("\nGo numbers sourced from bench-baseline.txt (Apple M4, arm64).")
    print("Install: pip install networkx python-louvain leidenalg")


if __name__ == "__main__":
    main()
