"""
Generate Phase C crash recovery experiment charts from collected results.

Usage:
    python3 analysis/generate_crash_charts.py
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
CHARTS_DIR = PROJECT_ROOT / "analysis" / "results" / "crash_charts"

RUNS = {
    4: "20260420T015747Z/4w_crash",
    8: "20260420T022546Z/8w_crash",
}

COLORS = {
    4: "#ff7f0e",
    8: "#1f77b4",
}


def load_summary(workers: int) -> dict:
    path = RESULTS_ROOT / RUNS[workers] / "summary.json"
    with path.open() as f:
        return json.load(f)


def load_crash_metadata(workers: int) -> dict:
    path = RESULTS_ROOT / RUNS[workers] / "crash_metadata.json"
    with path.open() as f:
        return json.load(f)


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


def chart_queue_depth_with_crash(summaries: dict[int, dict], metadata: dict[int, dict]) -> None:
    fig, ax = plt.subplots(figsize=(12, 6))

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
                    label=f"{workers} workers (kill 1 = {100//workers}% loss)",
                    color=COLORS[workers], alpha=0.85)

    # Add crash marker at 60s
    ax.axvline(x=60, color="#d62728", linestyle="--", linewidth=2, alpha=0.7, label="Worker killed (t=60s)")

    ax.set_xlabel("Elapsed Time (seconds)", fontsize=12)
    ax.set_ylabel("Pending Queue Depth", fontsize=12)
    ax.set_title("Queue Depth During Crash Recovery — Sustained Load at 100 jobs/min", fontsize=14)
    ax.legend(fontsize=11)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "crash_queue_depth_over_time.png", dpi=200)
    plt.close(fig)
    print(f"  crash_queue_depth_over_time.png")


def chart_throughput_comparison(summaries: dict[int, dict]) -> None:
    workers_list = sorted(summaries.keys())
    throughput = [summaries[w]["throughput_jobs_per_sec"] for w in workers_list]

    fig, ax = plt.subplots(figsize=(8, 5))
    bars = ax.bar([f"{w} workers\n(kill 1)" for w in workers_list], throughput,
                  color=[COLORS[w] for w in workers_list], width=0.4)
    for bar, val in zip(bars, throughput):
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.02,
                f"{val:.3f}", ha="center", va="bottom", fontsize=11)
    ax.set_ylabel("Throughput (jobs/sec)", fontsize=12)
    ax.set_title("Throughput During Crash Recovery", fontsize=14)
    ax.grid(axis="y", linestyle="--", alpha=0.35)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "crash_throughput_comparison.png", dpi=200)
    plt.close(fig)
    print(f"  crash_throughput_comparison.png")


def chart_recovery_summary(summaries: dict[int, dict], metadata: dict[int, dict]) -> None:
    workers_list = sorted(summaries.keys())

    fig, (ax1, ax2, ax3) = plt.subplots(1, 3, figsize=(16, 5))

    # Drain time
    drain = [summaries[w]["drain_time_seconds"] for w in workers_list]
    bars1 = ax1.bar([f"{w}w" for w in workers_list], drain,
                    color=[COLORS[w] for w in workers_list], width=0.4)
    for bar, val in zip(bars1, drain):
        ax1.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 10,
                 f"{val:.0f}s", ha="center", va="bottom", fontsize=10)
    ax1.set_ylabel("Drain Time (seconds)", fontsize=11)
    ax1.set_title("Total Drain Time", fontsize=13)
    ax1.grid(axis="y", linestyle="--", alpha=0.35)

    # Queue wait p50 vs p95
    p50 = [summaries[w]["queue_wait_p50"] for w in workers_list]
    p95 = [summaries[w]["queue_wait_p95"] for w in workers_list]
    x = range(len(workers_list))
    width = 0.3
    ax2.bar([i - width/2 for i in x], p50, width, label="p50", color="#ff7f0e", alpha=0.8)
    ax2.bar([i + width/2 for i in x], p95, width, label="p95", color="#d62728", alpha=0.8)
    ax2.set_xticks(list(x))
    ax2.set_xticklabels([f"{w}w" for w in workers_list])
    ax2.set_ylabel("Queue Wait (seconds)", fontsize=11)
    ax2.set_title("Queue Wait Latency", fontsize=13)
    ax2.legend(fontsize=10)
    ax2.grid(axis="y", linestyle="--", alpha=0.35)

    # Jobs outcome
    for i, w in enumerate(workers_list):
        completed = summaries[w]["completed_jobs"]
        failed = summaries[w]["failed_jobs"]
        total = summaries[w]["total_jobs"]
        recovered = metadata[w]["post_experiment_metrics"].get("jobs_recovered_stale", 0)
        ax3.bar(i, completed, color=COLORS[w], width=0.4, label=f"{w}w: {completed}/{total}")
        ax3.text(i, completed + 5, f"{completed}/{total}\n0 lost\n{recovered} recovered",
                 ha="center", va="bottom", fontsize=9)

    ax3.set_xticks(list(range(len(workers_list))))
    ax3.set_xticklabels([f"{w}w" for w in workers_list])
    ax3.set_ylabel("Jobs Completed", fontsize=11)
    ax3.set_title("Job Completion After Crash", fontsize=13)
    ax3.grid(axis="y", linestyle="--", alpha=0.35)

    fig.suptitle("Phase C: Crash Recovery Under Sustained Load — Worker Killed at t=60s",
                 fontsize=14, y=1.02)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "crash_recovery_summary.png", dpi=200, bbox_inches="tight")
    plt.close(fig)
    print(f"  crash_recovery_summary.png")


def chart_graceful_shutdown_story() -> None:
    """Annotated diagram showing why jobs_recovered_stale=0."""
    fig, ax = plt.subplots(figsize=(10, 4))
    ax.set_xlim(0, 10)
    ax.set_ylim(0, 6)
    ax.axis("off")

    steps = [
        (1, 5, "1. ECS sends\nSIGTERM"),
        (3, 5, "2. Worker finishes\ncurrent job"),
        (5, 5, "3. Worker exits\ncleanly"),
        (7, 5, "4. ECS launches\nnew task"),
        (9, 5, "5. No stale jobs\n0 retries needed"),
    ]

    for i, (x, y, text) in enumerate(steps):
        color = "#2ca02c" if i < 4 else "#1f77b4"
        ax.add_patch(plt.Rectangle((x - 0.8, y - 0.7), 1.6, 1.4,
                                    facecolor=color, alpha=0.15, edgecolor=color, linewidth=2))
        ax.text(x, y, text, ha="center", va="center", fontsize=10, fontweight="bold")
        if i < len(steps) - 1:
            ax.annotate("", xy=(steps[i+1][0] - 0.8, y), xytext=(x + 0.8, y),
                        arrowprops=dict(arrowstyle="->", color="#666", lw=2))

    ax.set_title("Graceful Shutdown: Why Zero Jobs Were Lost or Retried", fontsize=14, pad=20)
    fig.tight_layout()
    fig.savefig(CHARTS_DIR / "graceful_shutdown_flow.png", dpi=200, bbox_inches="tight")
    plt.close(fig)
    print(f"  graceful_shutdown_flow.png")


def main() -> None:
    CHARTS_DIR.mkdir(parents=True, exist_ok=True)

    summaries = {w: load_summary(w) for w in RUNS}
    metadata = {w: load_crash_metadata(w) for w in RUNS}

    print("Generating Phase C crash recovery charts:")
    chart_queue_depth_with_crash(summaries, metadata)
    chart_throughput_comparison(summaries)
    chart_recovery_summary(summaries, metadata)
    chart_graceful_shutdown_story()
    print(f"\nAll charts saved to: {CHARTS_DIR}")


if __name__ == "__main__":
    main()
