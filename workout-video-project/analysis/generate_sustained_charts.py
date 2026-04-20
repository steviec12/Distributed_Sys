"""
Generate Phase B sustained load experiment charts from collected results.

Usage:
    python3 analysis/generate_sustained_charts.py
"""

from __future__ import annotations

import csv
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

RESULTS_ROOT = PROJECT_ROOT / "analysis" / "results" / "sustained"
CHARTS_DIR = PROJECT_ROOT / "analysis" / "results" / "sustained_charts"

# Runs with metrics data (skip the first two without metrics)
RUNS = {
    2: "20260419T232815Z/2w_100r",
    4: "20260420T001158Z/4w_100r",
    6: "20260420T003344Z/6w_100r",
    8: "20260420T004754Z/8w_100r",
}

COLORS = {
    2: "#d62728",
    4: "#ff7f0e",
    6: "#2ca02c",
    8: "#1f77b4",
}


def load_summaries() -> dict[int, dict]:
    summaries = {}
    for workers, path in RUNS.items():
        summary_path = RESULTS_ROOT / path / "summary.json"
        with summary_path.open() as f:
            summaries[workers] = json.load(f)
    return summaries


def load_metrics(workers: int) -> list[dict]:
    path = RESULTS_ROOT / RUNS[workers] / "metrics_timeseries.csv"
    if not path.exists():
        return []
    rows = []
    with path.open("r", newline="", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            rows.append(row)
    return rows


def chart_queue_depth_over_time(summaries: dict[int, dict]) -> None:
    fig, ax = plt.subplots(figsize=(10, 6))

    for workers in sorted(RUNS.keys()):
        metrics = load_metrics(workers)
        if not metrics:
            continue

        elapsed = []
        pending = []
        for row in metrics:
            if row.get("error"):
                continue
            try:
                e = float(row.get("elapsed_seconds", 0))
                p = int(row.get("pending_queue_depth", 0))
                elapsed.append(e)
                pending.append(p)
            except (ValueError, TypeError):
                continue

        if elapsed:
            ax.plot(elapsed, pending, linewidth=1.5,
                    label=f"{workers} workers", color=COLORS[workers], alpha=0.85)

    ax.set_xlabel("Elapsed Time (seconds)", fontsize=12)
    ax.set_ylabel("Pending Queue Depth", fontsize=12)
    ax.set_title("Queue Depth Over Time — Sustained Load at 100 jobs/min", fontsize=14)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "queue_depth_over_time.png", dpi=200)
    plt.close(fig)
    print(f"  queue_depth_over_time.png")


def chart_throughput_comparison(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    throughput = [summaries[w]["throughput_jobs_per_sec"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    bars = ax.bar([str(w) for w in workers], throughput, color=[COLORS[w] for w in workers], width=0.5)
    for bar, val in zip(bars, throughput):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.02,
                f"{val:.3f}", ha="center", va="bottom", fontsize=10)
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Throughput (jobs/sec)", fontsize=12)
    ax.set_title("Sustained Throughput at 100 jobs/min", fontsize=14)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "sustained_throughput.png", dpi=200)
    plt.close(fig)
    print(f"  sustained_throughput.png")


def chart_drain_time_comparison(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    drain = [summaries[w]["drain_time_seconds"] for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    bars = ax.bar([str(w) for w in workers], drain, color=[COLORS[w] for w in workers], width=0.5)
    for bar, val in zip(bars, drain):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 20,
                f"{val:.0f}s", ha="center", va="bottom", fontsize=10)
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Drain Time (seconds)", fontsize=12)
    ax.set_title("Total Drain Time at 100 jobs/min", fontsize=14)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "sustained_drain_time.png", dpi=200)
    plt.close(fig)
    print(f"  sustained_drain_time.png")


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
    ax.set_title("Queue Wait Percentiles — Sustained Load at 100 jobs/min", fontsize=14)
    ax.set_xticks(workers)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "sustained_queue_wait_percentiles.png", dpi=200)
    plt.close(fig)
    print(f"  sustained_queue_wait_percentiles.png")


def chart_peak_vs_avg_queue(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    peak = [summaries[w]["peak_pending_queue_depth"] or 0 for w in workers]
    avg = [summaries[w]["avg_pending_queue_depth"] or 0 for w in workers]

    fig, ax = plt.subplots(figsize=(8, 5))
    x = range(len(workers))
    width = 0.35
    bars1 = ax.bar([i - width/2 for i in x], peak, width, label="Peak", color="#d62728", alpha=0.8)
    bars2 = ax.bar([i + width/2 for i in x], avg, width, label="Average", color="#ff7f0e", alpha=0.8)
    ax.set_xlabel("Worker Count", fontsize=12)
    ax.set_ylabel("Pending Queue Depth", fontsize=12)
    ax.set_title("Peak vs Average Queue Depth — Sustained Load at 100 jobs/min", fontsize=14)
    ax.set_xticks(x)
    ax.set_xticklabels([str(w) for w in workers])
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "sustained_peak_vs_avg_queue.png", dpi=200)
    plt.close(fig)
    print(f"  sustained_peak_vs_avg_queue.png")


def chart_combined_summary(summaries: dict[int, dict]) -> None:
    workers = sorted(summaries.keys())
    throughput = [summaries[w]["throughput_jobs_per_sec"] for w in workers]
    peak_q = [summaries[w]["peak_pending_queue_depth"] or 0 for w in workers]
    e2e_p50 = [summaries[w]["end_to_end_p50"] for w in workers]

    fig, (ax1, ax2, ax3) = plt.subplots(1, 3, figsize=(18, 5))

    ax1.plot(workers, throughput, marker="o", linewidth=2.5, markersize=8, color="#1f77b4")
    ax1.set_xlabel("Worker Count", fontsize=11)
    ax1.set_ylabel("Throughput (jobs/sec)", fontsize=11)
    ax1.set_title("Throughput", fontsize=13)
    ax1.set_xticks(workers)
    ax1.grid(axis="y", linestyle="--", alpha=0.35)

    ax2.bar([str(w) for w in workers], peak_q, color=[COLORS[w] for w in workers], width=0.5)
    ax2.set_xlabel("Worker Count", fontsize=11)
    ax2.set_ylabel("Peak Queue Depth", fontsize=11)
    ax2.set_title("Peak Queue Backlog", fontsize=13)
    ax2.grid(axis="y", linestyle="--", alpha=0.35)

    ax3.plot(workers, e2e_p50, marker="o", linewidth=2.5, markersize=8, color="#d62728")
    ax3.set_xlabel("Worker Count", fontsize=11)
    ax3.set_ylabel("End-to-End p50 (seconds)", fontsize=11)
    ax3.set_title("Median Latency", fontsize=13)
    ax3.set_xticks(workers)
    ax3.grid(axis="y", linestyle="--", alpha=0.35)

    fig.suptitle("Phase B: Sustained Load at 100 jobs/min — All Worker Counts Overloaded", fontsize=14, y=1.02)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "combined_sustained_summary.png", dpi=200, bbox_inches="tight")
    plt.close(fig)
    print(f"  combined_sustained_summary.png")


def main() -> None:
    CHARTS_DIR.mkdir(parents=True, exist_ok=True)
    summaries = load_summaries()

    print("Generating Phase B sustained load charts:")
    chart_queue_depth_over_time(summaries)
    chart_throughput_comparison(summaries)
    chart_drain_time_comparison(summaries)
    chart_queue_wait_percentiles(summaries)
    chart_peak_vs_avg_queue(summaries)
    chart_combined_summary(summaries)
    print(f"\nAll charts saved to: {CHARTS_DIR}")


if __name__ == "__main__":
    main()
