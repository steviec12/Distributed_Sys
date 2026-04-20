"""
Result collector for the Phase B sustained load experiment.

Two modes:
  poll-metrics  Poll /metrics every 500ms and write to CSV until killed.
  collect       Read job IDs, wait for all jobs to complete, compute stats,
                merge metrics time-series, and write output.

Usage:
    # Start metrics polling in background
    python3 analysis/run_sustained_load_experiment.py \
        --api-base-url http://<ALB> \
        --job-id-file ~/job_ids.txt \
        --worker-count 4 --rate 100 --label "4w_100r" --mode poll-metrics &

    # After Locust finishes, collect results
    python3 analysis/run_sustained_load_experiment.py \
        --api-base-url http://<ALB> \
        --job-id-file ~/job_ids.txt \
        --worker-count 4 --rate 100 --label "4w_100r" --mode collect
"""

from __future__ import annotations

import argparse
import csv
import json
import signal
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error, request


PROJECT_ROOT = Path(__file__).resolve().parent.parent
RESULTS_ROOT = PROJECT_ROOT / "analysis" / "results" / "sustained"
METRICS_LIVE_FILE = PROJECT_ROOT / "analysis" / "results" / "metrics_live_sustained.csv"

METRICS_FIELDNAMES = [
    "sampled_at", "elapsed_seconds",
    "jobs_submitted", "jobs_completed", "jobs_failed",
    "jobs_rejected", "jobs_recovered_stale", "jobs_recovered_failed",
    "jobs_poison_pill", "pending_queue_depth", "processing_queue_depth", "error",
]

JOB_FIELDNAMES = [
    "job_order", "job_id", "status", "file_name", "file_size_bytes",
    "created_at", "processing_started_at", "completed_at", "error",
    "queue_wait_s", "processing_time_s", "end_to_end_s",
    "duration_seconds", "simulated_analysis_seconds",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Phase B sustained load experiment tooling.")
    parser.add_argument("--api-base-url", required=True, help="API base URL (ALB endpoint).")
    parser.add_argument("--job-id-file", required=True, help="Path to file containing job IDs.")
    parser.add_argument("--worker-count", type=int, required=True, help="Number of workers for this run.")
    parser.add_argument("--rate", type=int, required=True, help="Target submission rate (jobs/min).")
    parser.add_argument("--label", default="", help="Label for output directory.")
    parser.add_argument("--mode", choices=["poll-metrics", "collect"], default="collect")
    parser.add_argument("--metrics-interval", type=float, default=0.5)
    parser.add_argument("--poll-interval", type=float, default=1.0)
    parser.add_argument("--completion-timeout", type=float, default=900.0,
                        help="Max seconds to wait for all jobs after Locust finishes.")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if args.mode == "poll-metrics":
        run_metrics_poller(args)
    else:
        run_collector(args)


def run_metrics_poller(args: argparse.Namespace) -> None:
    import threading
    metrics_path = METRICS_LIVE_FILE
    metrics_path.parent.mkdir(parents=True, exist_ok=True)

    stop = threading.Event()

    def handle_signal(signum, frame):
        stop.set()

    signal.signal(signal.SIGTERM, handle_signal)
    signal.signal(signal.SIGINT, handle_signal)

    start_time = time.monotonic()

    with metrics_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=METRICS_FIELDNAMES, extrasaction="ignore")
        writer.writeheader()
        handle.flush()

        print(f"Metrics poller started, writing to {metrics_path}")
        while not stop.is_set():
            elapsed = round(time.monotonic() - start_time, 1)
            row: dict[str, Any] = {"sampled_at": utc_now_iso(), "elapsed_seconds": elapsed}
            try:
                row.update(api_get_json(args.api_base_url, "/metrics"))
                row["error"] = ""
            except Exception as exc:
                row["error"] = str(exc)
            writer.writerow(row)
            handle.flush()
            stop.wait(args.metrics_interval)

    print("Metrics poller stopped.")


def run_collector(args: argparse.Namespace) -> None:
    job_ids = read_job_ids(args.job_id_file)
    if not job_ids:
        raise SystemExit("No job IDs found. Run the Locust load driver first.")

    label = args.label or f"{args.worker_count}w_{args.rate}r"
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    result_dir = RESULTS_ROOT / run_id / label
    result_dir.mkdir(parents=True, exist_ok=True)

    print(f"Collecting results for {len(job_ids)} jobs ({args.worker_count} workers, {args.rate} jobs/min).")

    latest_jobs = wait_for_terminal_jobs(
        api_base_url=args.api_base_url,
        job_ids=job_ids,
        poll_interval=args.poll_interval,
        completion_timeout=args.completion_timeout,
    )

    final_metrics = {}
    try:
        final_metrics = api_get_json(args.api_base_url, "/metrics")
    except Exception as exc:
        final_metrics = {"metrics_error": str(exc)}

    metrics_rows = read_metrics_csv(METRICS_LIVE_FILE)
    job_rows = build_job_rows(latest_jobs, job_ids)
    summary = build_summary(args, job_rows, metrics_rows, final_metrics, result_dir)

    write_csv(result_dir / "job_results.csv", job_rows, JOB_FIELDNAMES)
    write_csv(result_dir / "metrics_timeseries.csv", metrics_rows, METRICS_FIELDNAMES)
    write_json(result_dir / "summary.json", summary)

    print_summary(summary)
    print(f"\nArtifacts written to: {result_dir}")


def read_job_ids(path: str) -> list[str]:
    p = Path(path)
    if not p.exists():
        return []
    lines = p.read_text().strip().splitlines()
    return [line.strip() for line in lines if line.strip()]


def read_metrics_csv(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    rows: list[dict[str, Any]] = []
    with path.open("r", newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        for row in reader:
            rows.append(row)
    return rows


def wait_for_terminal_jobs(
    api_base_url: str,
    job_ids: list[str],
    poll_interval: float,
    completion_timeout: float,
) -> dict[str, dict[str, Any]]:
    print("Waiting for all jobs to reach terminal state...")
    deadline = time.monotonic() + completion_timeout
    latest_jobs: dict[str, dict[str, Any]] = {}
    terminal_count = 0

    while time.monotonic() < deadline:
        latest_jobs = fetch_jobs(api_base_url, job_ids)
        terminal_count = sum(
            1 for job in latest_jobs.values()
            if job.get("status") in {"completed", "failed"}
        )
        print(f"  {terminal_count}/{len(job_ids)} jobs terminal", end="\r")
        if terminal_count == len(job_ids):
            print(f"  {terminal_count}/{len(job_ids)} jobs terminal — done.")
            return latest_jobs
        time.sleep(poll_interval)

    print(f"\n  WARNING: only {terminal_count}/{len(job_ids)} reached terminal state before timeout.")
    return latest_jobs


def fetch_jobs(api_base_url: str, job_ids: list[str]) -> dict[str, dict[str, Any]]:
    result = {}
    for job_id in job_ids:
        try:
            result[job_id] = api_get_json(api_base_url, f"/jobs/{job_id}")
        except Exception:
            pass
    return result


def build_job_rows(latest_jobs: dict[str, dict[str, Any]], job_ids: list[str]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for order, job_id in enumerate(job_ids, start=1):
        job = latest_jobs.get(job_id, {})
        rows.append({
            "job_order": order,
            "job_id": job_id,
            "status": job.get("status", "unknown"),
            "file_name": job.get("file_name"),
            "file_size_bytes": job.get("file_size_bytes"),
            "created_at": job.get("created_at"),
            "processing_started_at": job.get("processing_started_at"),
            "completed_at": job.get("completed_at"),
            "error": job.get("error"),
            "queue_wait_s": seconds_between(job.get("created_at"), job.get("processing_started_at")),
            "processing_time_s": seconds_between(job.get("processing_started_at"), job.get("completed_at")),
            "end_to_end_s": seconds_between(job.get("created_at"), job.get("completed_at")),
            "duration_seconds": nested_float(job, "result", "duration_seconds"),
            "simulated_analysis_seconds": nested_float(job, "result", "simulated_analysis_seconds"),
        })
    return rows


def build_summary(
    args: argparse.Namespace,
    job_rows: list[dict[str, Any]],
    metrics_rows: list[dict[str, Any]],
    final_metrics: dict[str, Any],
    result_dir: Path,
) -> dict[str, Any]:
    completed = [r for r in job_rows if r["status"] == "completed"]
    failed = [r for r in job_rows if r["status"] == "failed"]

    queue_waits = [r["queue_wait_s"] for r in completed if r["queue_wait_s"] is not None]
    processing_times = [r["processing_time_s"] for r in completed if r["processing_time_s"] is not None]
    end_to_ends = [r["end_to_end_s"] for r in completed if r["end_to_end_s"] is not None]

    created_times = [parse_iso8601(r["created_at"]) for r in job_rows if r.get("created_at")]
    completed_times = [parse_iso8601(r["completed_at"]) for r in completed if r.get("completed_at")]

    drain_time = None
    throughput = None
    if created_times and completed_times:
        drain_time = round((max(completed_times) - min(created_times)).total_seconds(), 3)
        if drain_time > 0:
            throughput = round(len(completed) / drain_time, 3)

    peak_pending = compute_peak(metrics_rows, "pending_queue_depth")
    peak_processing = compute_peak(metrics_rows, "processing_queue_depth")
    avg_pending = compute_avg(metrics_rows, "pending_queue_depth")

    return {
        "experiment": "phase_b_sustained_load",
        "run_at": utc_now_iso(),
        "worker_count": args.worker_count,
        "target_rate_jobs_per_min": args.rate,
        "total_jobs": len(job_rows),
        "completed_jobs": len(completed),
        "failed_jobs": len(failed),
        "drain_time_seconds": drain_time,
        "throughput_jobs_per_sec": throughput,
        "queue_wait_p50": percentile(queue_waits, 50),
        "queue_wait_p95": percentile(queue_waits, 95),
        "queue_wait_p99": percentile(queue_waits, 99),
        "processing_time_p50": percentile(processing_times, 50),
        "processing_time_p95": percentile(processing_times, 95),
        "processing_time_p99": percentile(processing_times, 99),
        "end_to_end_p50": percentile(end_to_ends, 50),
        "end_to_end_p95": percentile(end_to_ends, 95),
        "end_to_end_p99": percentile(end_to_ends, 99),
        "peak_pending_queue_depth": peak_pending,
        "peak_processing_queue_depth": peak_processing,
        "avg_pending_queue_depth": avg_pending,
        "metrics_samples_collected": len(metrics_rows),
        "final_metrics": final_metrics,
        "result_dir": str(result_dir),
    }


def compute_peak(metrics_rows: list[dict[str, Any]], key: str) -> int | None:
    values = []
    for row in metrics_rows:
        if row.get("error"):
            continue
        try:
            values.append(int(row.get(key, 0)))
        except (ValueError, TypeError):
            continue
    return max(values) if values else None


def compute_avg(metrics_rows: list[dict[str, Any]], key: str) -> float | None:
    values = []
    for row in metrics_rows:
        if row.get("error"):
            continue
        try:
            values.append(int(row.get(key, 0)))
        except (ValueError, TypeError):
            continue
    if not values:
        return None
    return round(sum(values) / len(values), 2)


def print_summary(summary: dict[str, Any]) -> None:
    print(f"\n{'=' * 60}")
    print(f"Phase B Results — {summary['worker_count']} workers, {summary['target_rate_jobs_per_min']} jobs/min")
    print(f"{'=' * 60}")
    print(f"Jobs:       {summary['completed_jobs']} completed, {summary['failed_jobs']} failed / {summary['total_jobs']} total")
    print(f"Drain time: {summary['drain_time_seconds']}s")
    print(f"Throughput: {summary['throughput_jobs_per_sec']} jobs/sec")
    print(f"Queue wait:  p50={summary['queue_wait_p50']}s  p95={summary['queue_wait_p95']}s  p99={summary['queue_wait_p99']}s")
    print(f"Processing:  p50={summary['processing_time_p50']}s  p95={summary['processing_time_p95']}s  p99={summary['processing_time_p99']}s")
    print(f"End-to-end:  p50={summary['end_to_end_p50']}s  p95={summary['end_to_end_p95']}s  p99={summary['end_to_end_p99']}s")
    print(f"Peak pending queue:    {summary['peak_pending_queue_depth']}")
    print(f"Avg pending queue:     {summary['avg_pending_queue_depth']}")
    print(f"Metrics samples:       {summary['metrics_samples_collected']}")


def percentile(data: list[float], pct: int) -> float | None:
    if not data:
        return None
    sorted_data = sorted(data)
    k = (len(sorted_data) - 1) * pct / 100
    f = int(k)
    c = f + 1
    if c >= len(sorted_data):
        return round(sorted_data[f], 3)
    return round(sorted_data[f] + (k - f) * (sorted_data[c] - sorted_data[f]), 3)


def write_csv(path: Path, rows: list[dict[str, Any]], fieldnames: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames, extrasaction="ignore")
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2), encoding="utf-8")


def api_get_json(api_base_url: str, path: str) -> dict[str, Any]:
    url = f"{api_base_url.rstrip('/')}{path}"
    try:
        with request.urlopen(url, timeout=10) as response:
            return json.loads(response.read().decode("utf-8"))
    except error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"GET {url} failed with HTTP {exc.code}: {body}") from exc


def seconds_between(start: str | None, end: str | None) -> float | None:
    if not start or not end:
        return None
    return round((parse_iso8601(end) - parse_iso8601(start)).total_seconds(), 3)


def parse_iso8601(value: str) -> datetime:
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)


def nested_float(data: dict[str, Any], *keys: str) -> float | None:
    current: Any = data
    for key in keys:
        if not isinstance(current, dict):
            return None
        current = current.get(key)
    if current is None:
        return None
    return float(current)


def utc_now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%f")[:-3] + "Z"


if __name__ == "__main__":
    main()
