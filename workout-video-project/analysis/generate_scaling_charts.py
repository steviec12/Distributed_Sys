"""
Generate Phase A scaling experiment charts from collected results.

Usage:
    python3 analysis/generate_scaling_charts.py
"""

from __future__ import annotations

import json
import os
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
MPLCONFIGDIR = PROJECT_ROOT / ".cache" / "matplotlib"
MPLCONFIGDIR.mkdir(parents=True, exist_ok=True)
os.environ.setdefault("MPLCONFIGDIR", str(MPLCONFIGDIR))

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

RESULTS_ROOT = PROJECT_ROOT / "analysis" / "results" / "scaling"
CHARTS_DIR = PROJECT_ROOT / "analysis" / "results" / "scaling_charts"

# EC2-based runs only (skip Mac-based runs with drain > 800s)
EC2_RUNS = {
    1: "20260419T041008Z/1_workers",
    2: "20260419T042444Z/2_workers",
    4: "20260419T043931Z/4_workers",
    8: "20260419T044608Z/8_workers",
}


def load_summaries() -> dict[int, dict]:
    summaries = {}
    for workers, path in EC2_RUNS.items():
        summary_path = RESULTS_ROOT / path / "summary.json"
        with summary_path.open() as f:
            summaries[workers] = json.load(f)
    return summaries


def chart_throughput(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    throughput = [summaries[w]["throughput_jobs_per_sec"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(workers, throughput, marker="o", linewidth=2.5, markersize=8, color="#1f77b4")
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Throughput (jobs/sec)", fontsize=12)
    ax.set_title("Throughput vs Worker Count", fontsize=14)
    ax.set_xticks(workers)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "throughput_vs_workers.png", dpi=200)
    plt.close(fig)
    print(f"  throughput_vs_workers.png")


def chart_drain_time(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    drain = [summaries[w]["drain_time_seconds"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    bars = ax.bar([str(w) for w in workers], drain, color="#2ca02c", width=0.5)
    for bar, val in zip(bars, drain):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 10,
                f"{val:.0f}s", ha="center", va="bottom", fontsize=10)
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Drain Time (seconds)", fontsize=12)
    ax.set_title("Total Drain Time vs Worker Count", fontsize=14)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "drain_time_vs_workers.png", dpi=200)
    plt.close(fig)
    print(f"  drain_time_vs_workers.png")


def chart_queue_wait_percentiles(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    p50 = [summaries[w]["queue_wait_p50"] for w in workers]
    p95 = [summaries[w]["queue_wait_p95"] for w in workers]
    p99 = [summaries[w]["queue_wait_p99"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(workers, p50, marker="o", linewidth=2, markersize=7, label="p50", color="#1f77b4")
    ax.plot(workers, p95, marker="s", linewidth=2, markersize=7, label="p95", color="#ff7f0e")
    ax.plot(workers, p99, marker="^", linewidth=2, markersize=7, label="p99", color="#d62728")
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Queue Wait Time (seconds)", fontsize=12)
    ax.set_title("Queue Wait Latency Percentiles vs Worker Count", fontsize=14)
    ax.set_xticks(workers)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "queue_wait_percentiles.png", dpi=200)
    plt.close(fig)
    print(f"  queue_wait_percentiles.png")


def chart_end_to_end_percentiles(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    p50 = [summaries[w]["end_to_end_p50"] for w in workers]
    p95 = [summaries[w]["end_to_end_p95"] for w in workers]
    p99 = [summaries[w]["end_to_end_p99"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    ax.plot(workers, p50, marker="o", linewidth=2, markersize=7, label="p50", color="#1f77b4")
    ax.plot(workers, p95, marker="s", linewidth=2, markersize=7, label="p95", color="#ff7f0e")
    ax.plot(workers, p99, marker="^", linewidth=2, markersize=7, label="p99", color="#d62728")
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("End-to-End Time (seconds)", fontsize=12)
    ax.set_title("End-to-End Latency Percentiles vs Worker Count", fontsize=14)
    ax.set_xticks(workers)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "end_to_end_percentiles.png", dpi=200)
    plt.close(fig)
    print(f"  end_to_end_percentiles.png")


def chart_combined_summary(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    throughput = [summaries[w]["throughput_jobs_per_sec"] for w in workers]
    queue_p50 = [summaries[w]["queue_wait_p50"] for w in workers]
    e2e_p50 = [summaries[w]["end_to_end_p50"] for w in workers]

    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(14, 5))

    # Left: throughput
    ax1.plot(workers, throughput, marker="o", linewidth=2.5, markersize=8, color="#1f77b4")
    ax1.set_xlabel("Worker Count", fontsize=12)
    ax1.set_ylabel("Throughput (jobs/sec)", fontsize=12)
    ax1.set_title("Throughput Saturation", fontsize=13)
    ax1.set_xticks(workers)
    ax1.grid(axis="y", linestyle="--", alpha=0.35)

    # Right: latency
    ax2.plot(workers, queue_p50, marker="o", linewidth=2, markersize=7, label="Queue Wait p50", color="#ff7f0e")
    ax2.plot(workers, e2e_p50, marker="s", linewidth=2, markersize=7, label="End-to-End p50", color="#d62728")
    ax2.set_xlabel("Worker Count", fontsize=12)
    ax2.set_ylabel("Latency (seconds)", fontsize=12)
    ax2.set_title("Median Latency Reduction", fontsize=13)
    ax2.set_xticks(workers)
    ax2.legend(fontsize=11)
    ax2.grid(axis="y", linestyle="--", alpha=0.35)

    fig.suptitle("Phase A: Capacity Profiling — 100 Jobs, Mixed Video Workload", fontsize=14, y=1.02)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "combined_scaling_summary.png", dpi=200, bbox_inches="tight")
    plt.close(fig)
    print(f"  combined_scaling_summary.png")


def main() -> None:
    CHARTS_DIR.mkdir(parents=True, exist_ok=True)
    summaries = load_summaries()

    print("Generating Phase A scaling charts:")
    chart_throughput(summaries)
    chart_drain_time(summaries)
    chart_queue_wait_percentiles(summaries)
    chart_end_to_end_percentiles(summaries)
    chart_combined_summary(summaries)
    print(f"\nAll charts saved to: {CHARTS_DIR}")


if __name__ == "__main__":
    main()
