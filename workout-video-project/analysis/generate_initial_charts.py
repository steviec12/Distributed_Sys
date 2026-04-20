from __future__ import annotations

import csv
import os
from collections import defaultdict
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent.parent
MPLCONFIGDIR = PROJECT_ROOT / ".cache" / "matplotlib"
MPLCONFIGDIR.mkdir(parents=True, exist_ok=True)
os.environ.setdefault("MPLCONFIGDIR", str(MPLCONFIGDIR))

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt


DATA_FILE = PROJECT_ROOT / "analysis" / "preliminary_burst_results.csv"
RESULTS_DIR = PROJECT_ROOT / "results"


def load_rows() -> list[dict[str, str]]:
    with DATA_FILE.open(newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        return list(reader)


def pivot_series(rows: list[dict[str, str]], value_key: str) -> tuple[list[int], dict[str, list[float]]]:
    grouped: dict[str, dict[int, float]] = defaultdict(dict)
    job_orders = sorted({int(row["job_order"]) for row in rows})

    for row in rows:
        grouped[row["config"]][int(row["job_order"])] = float(row[value_key])

    series = {
        config: [grouped[config][job_order] for job_order in job_orders]
        for config in sorted(grouped.keys())
    }
    return job_orders, series


def make_line_chart(
    job_orders: list[int],
    series: dict[str, list[float]],
    title: str,
    y_label: str,
    output_path: Path,
) -> None:
    plt.figure(figsize=(8, 5))
    colors = {
        "1 worker": "#1f77b4",
        "2 workers": "#ff7f0e",
    }

    for label, values in series.items():
        plt.plot(
            job_orders,
            values,
            marker="o",
            linewidth=2.5,
            markersize=7,
            label=label,
            color=colors.get(label),
        )

    plt.xticks(job_orders)
    plt.xlabel("Job Order")
    plt.ylabel(y_label)
    plt.title(title)
    plt.grid(axis="y", linestyle="--", alpha=0.35)
    plt.legend()
    plt.tight_layout()
    plt.savefig(output_path, dpi=200)
    plt.close()


def main() -> None:
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    rows = load_rows()

    job_orders, queue_wait_series = pivot_series(rows, "queue_wait_s")
    make_line_chart(
        job_orders,
        queue_wait_series,
        title="Queue Wait Time by Job Order",
        y_label="Queue Wait Time (seconds)",
        output_path=RESULTS_DIR / "queue_wait_time_by_job_order.png",
    )

    job_orders, end_to_end_series = pivot_series(rows, "end_to_end_s")
    make_line_chart(
        job_orders,
        end_to_end_series,
        title="End-to-End Completion Time by Job Order",
        y_label="Completion Time (seconds)",
        output_path=RESULTS_DIR / "end_to_end_completion_time_by_job_order.png",
    )

    print("Generated charts:")
    print(f"- {RESULTS_DIR / 'queue_wait_time_by_job_order.png'}")
    print(f"- {RESULTS_DIR / 'end_to_end_completion_time_by_job_order.png'}")


if __name__ == "__main__":
    main()
