#!/usr/bin/env python3

import csv
import json
import math
import os
from collections import defaultdict
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


RESULTS_DIR = Path("/Users/stevi/Documents/web-service-gin/hw10/results")
OUTPUT_DIR = RESULTS_DIR / "graphs"
RATIO_ORDER = ["99/1", "90/10", "50/50", "10/90"]
RATIO_LABELS = {
    "99/1": "99% read / 1% write",
    "90/10": "90% read / 10% write",
    "50/50": "50% read / 50% write",
    "10/90": "10% read / 90% write",
}


def parse_run_label(run_dir: Path) -> str:
    relative = run_dir.relative_to(RESULTS_DIR)
    parts = relative.parts
    if len(parts) == 2 and parts[0].startswith("leader_follower-"):
        return parts[0]
    if len(parts) == 1 and parts[0].startswith("leaderless-"):
        return "leaderless-w5-r1"
    raise ValueError(f"unrecognized run directory layout: {run_dir}")


def discover_runs():
    runs = []

    for config_path in RESULTS_DIR.glob("leader_follower-*/*/config.json"):
        run_dir = config_path.parent
        runs.append(run_dir)

    for config_path in RESULTS_DIR.glob("leaderless-*/config.json"):
        run_dir = config_path.parent
        runs.append(run_dir)

    return sorted(runs)


def load_requests(csv_path: Path):
    read_latencies = []
    write_latencies = []
    read_after_write = []

    with csv_path.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            latency = row.get("latency_ms")
            if row["op"] == "read" and latency:
                read_latencies.append(float(latency))
                if row.get("read_after_write_ms"):
                    read_after_write.append(float(row["read_after_write_ms"]))
            elif row["op"] == "write" and latency:
                write_latencies.append(float(latency))

    return {
        "read_latency_ms": read_latencies,
        "write_latency_ms": write_latencies,
        "read_after_write_ms": read_after_write,
    }


def load_all_data():
    grouped = defaultdict(dict)
    summary_rows = []

    for run_dir in discover_runs():
        config = json.loads((run_dir / "config.json").read_text())
        summary = json.loads((run_dir / "summary.json").read_text())
        requests = load_requests(run_dir / "requests.csv")

        ratio = config["ratio"]
        mode_label = parse_run_label(run_dir)
        grouped[mode_label][ratio] = {
            "config": config,
            "summary": summary,
            "requests": requests,
            "dir": str(run_dir),
        }

        summary_rows.append(
            {
                "mode_label": mode_label,
                "ratio": ratio,
                "run_dir": str(run_dir),
                "total_reads": summary["total_reads"],
                "total_writes": summary["total_writes"],
                "eligible_reads_for_staleness": summary["eligible_reads_for_staleness"],
                "stale_reads_on_followers_or_non_coordinators": summary["stale_reads_on_followers_or_non_coordinators"],
                "stale_read_rate_on_eligible_reads": summary["stale_read_rate_on_eligible_reads"],
                "read_p95_ms": summary["read_latency_ms"]["p95"],
                "write_p95_ms": summary["write_latency_ms"]["p95"],
                "interval_p95_ms": summary["read_after_write_ms"]["p95"],
            }
        )

    return grouped, summary_rows


def metric_title(metric):
    if metric == "read_latency_ms":
        return "Read latency"
    if metric == "write_latency_ms":
        return "Write latency"
    return "Read-after-write interval"


def choose_bins(values):
    if not values:
        return 10
    return min(80, max(25, int(math.sqrt(len(values)))))


def plot_distribution_grid(mode_label, ratio_map):
    fig, axes = plt.subplots(3, 4, figsize=(22, 14), constrained_layout=True)
    metrics = ["read_latency_ms", "write_latency_ms", "read_after_write_ms"]
    colors = {
        "read_latency_ms": "#2b6cb0",
        "write_latency_ms": "#c05621",
        "read_after_write_ms": "#2f855a",
    }

    for row_idx, metric in enumerate(metrics):
        for col_idx, ratio in enumerate(RATIO_ORDER):
            ax = axes[row_idx][col_idx]
            run = ratio_map.get(ratio)
            ax.set_title(RATIO_LABELS[ratio], fontsize=11)
            ax.set_ylabel(metric_title(metric) if col_idx == 0 else "")
            ax.set_xlabel("Milliseconds")

            if run is None:
                ax.text(0.5, 0.5, "missing run", ha="center", va="center")
                ax.set_axis_off()
                continue

            values = run["requests"][metric]
            if not values:
                ax.text(0.5, 0.5, "no data", ha="center", va="center")
                ax.set_axis_off()
                continue

            ax.hist(
                values,
                bins=choose_bins(values),
                color=colors[metric],
                alpha=0.8,
                edgecolor="white",
                linewidth=0.4,
            )
            ax.set_xscale("log")
            ax.grid(True, which="both", axis="y", alpha=0.2)

            if metric == "read_latency_ms":
                p95 = run["summary"]["read_latency_ms"]["p95"]
                p99 = run["summary"]["read_latency_ms"]["p99"]
            elif metric == "write_latency_ms":
                p95 = run["summary"]["write_latency_ms"]["p95"]
                p99 = run["summary"]["write_latency_ms"]["p99"]
            else:
                p95 = run["summary"]["read_after_write_ms"]["p95"]
                p99 = run["summary"]["read_after_write_ms"]["p99"]

            if p95 is not None:
                ax.axvline(p95, color="black", linestyle="--", linewidth=1, alpha=0.8, label="p95")
            if p99 is not None:
                ax.axvline(p99, color="black", linestyle=":", linewidth=1, alpha=0.8, label="p99")
            if row_idx == 0 and col_idx == 0:
                ax.legend(frameon=False, fontsize=9)

    fig.suptitle(f"{mode_label}: request and timing distributions", fontsize=16)
    output_path = OUTPUT_DIR / f"{mode_label}-distributions.png"
    fig.savefig(output_path, dpi=180)
    plt.close(fig)
    return output_path


def plot_stale_summary(summary_rows):
    fig, ax = plt.subplots(figsize=(14, 7), constrained_layout=True)
    labels = []
    values = []
    colors = []

    palette = {
        "leader_follower-w5-r1": "#4a5568",
        "leader_follower-w1-r5": "#d69e2e",
        "leader_follower-w3-r3": "#2b6cb0",
        "leaderless-w5-r1": "#2f855a",
    }

    summary_rows = sorted(summary_rows, key=lambda row: (row["mode_label"], RATIO_ORDER.index(row["ratio"])))
    for row in summary_rows:
        labels.append(f'{row["mode_label"]}\n{row["ratio"]}')
        rate = row["stale_read_rate_on_eligible_reads"]
        values.append(0.0 if rate is None else float(rate))
        colors.append(palette.get(row["mode_label"], "#718096"))

    ax.bar(range(len(labels)), values, color=colors)
    ax.set_xticks(range(len(labels)))
    ax.set_xticklabels(labels, rotation=45, ha="right")
    ax.set_ylabel("Stale read rate")
    ax.set_title("Stale read rate on follower/non-coordinator reads")
    ax.grid(True, axis="y", alpha=0.2)

    output_path = OUTPUT_DIR / "stale-read-rate-summary.png"
    fig.savefig(output_path, dpi=180)
    plt.close(fig)
    return output_path


def write_summary_csv(summary_rows):
    output_path = OUTPUT_DIR / "run-summary.csv"
    fieldnames = [
        "mode_label",
        "ratio",
        "run_dir",
        "total_reads",
        "total_writes",
        "eligible_reads_for_staleness",
        "stale_reads_on_followers_or_non_coordinators",
        "stale_read_rate_on_eligible_reads",
        "read_p95_ms",
        "write_p95_ms",
        "interval_p95_ms",
    ]
    with output_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(summary_rows)
    return output_path


def main():
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    grouped, summary_rows = load_all_data()
    generated = []

    for mode_label, ratio_map in sorted(grouped.items()):
        generated.append(plot_distribution_grid(mode_label, ratio_map))

    generated.append(plot_stale_summary(summary_rows))
    generated.append(write_summary_csv(summary_rows))

    print("Generated:")
    for path in generated:
        print(path)


if __name__ == "__main__":
    main()
