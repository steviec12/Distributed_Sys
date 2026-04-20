from __future__ import annotations

import argparse
import csv
import json
import subprocess
import threading
import time
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error, request

from upload_client import submit_video_job


PROJECT_ROOT = Path(__file__).resolve().parent.parent
RESULTS_ROOT = PROJECT_ROOT / "analysis" / "results" / "crash_recovery"
DEFAULT_API_BASE_URL = "http://localhost:8080"


@dataclass
class ExperimentState:
    job_ids: list[str]
    worker_killed: bool
    worker_killed_at: str | None
    worker_killed_after_job_id: str | None
    recovery_detected_at: str | None
    worker_restarted_at: str | None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Run the Phase 4 crash-recovery experiment: submit a burst of jobs, "
            "kill the worker mid-processing, wait for the reaper to requeue stale work, "
            "restart the worker, and record results."
        )
    )
    parser.add_argument("--video-path", required=True, help="Absolute path to the video file to upload.")
    parser.add_argument("--burst-size", type=int, default=20, help="Number of jobs to submit in the burst.")
    parser.add_argument("--api-base-url", default=DEFAULT_API_BASE_URL, help="API base URL.")
    parser.add_argument(
        "--poll-interval",
        type=float,
        default=0.2,
        help="Seconds between job status polling attempts.",
    )
    parser.add_argument(
        "--metrics-interval",
        type=float,
        default=0.5,
        help="Seconds between /metrics samples.",
    )
    parser.add_argument(
        "--kill-timeout",
        type=float,
        default=10.0,
        help="Max seconds to wait for a job to enter in_progress before killing the worker.",
    )
    parser.add_argument(
        "--recovery-timeout",
        type=float,
        default=30.0,
        help="Max seconds to wait for stale-job recovery to be observed.",
    )
    parser.add_argument(
        "--completion-timeout",
        type=float,
        default=120.0,
        help="Max seconds to wait for all jobs to reach a terminal state after restart.",
    )
    parser.add_argument(
        "--skip-flush",
        action="store_true",
        help="Do not FLUSHDB before the experiment.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    video_path = Path(args.video_path).expanduser().resolve()
    if not video_path.exists():
        raise SystemExit(f"Video file does not exist: {video_path}")

    ensure_api_healthy(args.api_base_url)
    ensure_worker_started()
    if not args.skip_flush:
        flush_redis()

    result_dir = create_result_dir()
    metrics_rows: list[dict[str, Any]] = []
    metrics_stop = threading.Event()
    metrics_thread = threading.Thread(
        target=poll_metrics_forever,
        args=(args.api_base_url, args.metrics_interval, metrics_rows, metrics_stop),
        daemon=True,
    )
    metrics_thread.start()

    state = ExperimentState(
        job_ids=[],
        worker_killed=False,
        worker_killed_at=None,
        worker_killed_after_job_id=None,
        recovery_detected_at=None,
        worker_restarted_at=None,
    )

    latest_jobs: dict[str, dict[str, Any]] = {}
    final_metrics: dict[str, Any] | None = None
    notes: list[str] = []

    try:
        print(f"Submitting {args.burst_size} jobs using {video_path}...")
        state.job_ids = submit_burst(args.api_base_url, video_path, args.burst_size)
        print(f"Submitted {len(state.job_ids)} jobs.")

        latest_jobs = wait_for_in_progress_then_kill_worker(
            api_base_url=args.api_base_url,
            job_ids=state.job_ids,
            poll_interval=args.poll_interval,
            kill_timeout=args.kill_timeout,
            state=state,
        )

        latest_jobs = wait_for_recovery_signal(
            api_base_url=args.api_base_url,
            job_ids=state.job_ids,
            poll_interval=args.poll_interval,
            recovery_timeout=args.recovery_timeout,
            state=state,
        )

        restart_worker()
        state.worker_restarted_at = utc_now_iso()
        notes.append("Worker restarted after recovery wait.")

        latest_jobs = wait_for_terminal_jobs(
            api_base_url=args.api_base_url,
            job_ids=state.job_ids,
            poll_interval=args.poll_interval,
            completion_timeout=args.completion_timeout,
        )

        final_metrics = api_get_json(args.api_base_url, "/metrics")
    finally:
        metrics_stop.set()
        metrics_thread.join(timeout=2.0)
        if state.worker_killed and state.worker_restarted_at is None:
            try:
                restart_worker()
                state.worker_restarted_at = utc_now_iso()
                notes.append("Worker restarted during cleanup.")
            except subprocess.CalledProcessError as exc:
                notes.append(f"Worker restart during cleanup failed: {exc.stderr.strip() or exc.stdout.strip()}")

    if not latest_jobs:
        latest_jobs = fetch_jobs(args.api_base_url, state.job_ids)
    if final_metrics is None:
        try:
            final_metrics = api_get_json(args.api_base_url, "/metrics")
        except Exception as exc:  # pragma: no cover - best effort
            final_metrics = {"metrics_error": str(exc)}

    job_rows = build_job_rows(latest_jobs, state.job_ids)
    summary = build_summary(
        args=args,
        state=state,
        job_rows=job_rows,
        final_metrics=final_metrics,
        result_dir=result_dir,
        notes=notes,
    )

    write_job_results_csv(result_dir / "job_results.csv", job_rows)
    write_metrics_csv(result_dir / "metrics_timeseries.csv", metrics_rows)
    write_json(result_dir / "summary.json", summary)
    write_summary_markdown(result_dir / "summary.md", summary)

    print("")
    print("Crash-recovery experiment complete.")
    print(f"Artifacts written to: {result_dir}")
    print(f"- {result_dir / 'job_results.csv'}")
    print(f"- {result_dir / 'metrics_timeseries.csv'}")
    print(f"- {result_dir / 'summary.json'}")
    print(f"- {result_dir / 'summary.md'}")


def ensure_api_healthy(api_base_url: str) -> None:
    response = api_get_json(api_base_url, "/health")
    if response.get("status") != "ok":
        raise RuntimeError(f"API is not healthy: {response}")


def ensure_worker_started() -> None:
    run_compose_command(["start", "worker", "reaper", "api", "redis"])


def flush_redis() -> None:
    print("Flushing Redis so the experiment starts from a clean state...")
    run_compose_command(["exec", "-T", "redis", "redis-cli", "FLUSHDB"])


def submit_burst(api_base_url: str, video_path: Path, burst_size: int) -> list[str]:
    job_ids: list[str] = []
    for index in range(1, burst_size + 1):
        payload = submit_video_job(api_base_url, video_path)
        job_id = payload["job_id"]
        job_ids.append(job_id)
        print(f"  submitted job {index}/{burst_size}: {job_id}")
    return job_ids


def wait_for_in_progress_then_kill_worker(
    api_base_url: str,
    job_ids: list[str],
    poll_interval: float,
    kill_timeout: float,
    state: ExperimentState,
) -> dict[str, dict[str, Any]]:
    print("Waiting for a job to enter in_progress, then killing the worker...")
    deadline = time.monotonic() + kill_timeout
    latest_jobs: dict[str, dict[str, Any]] = {}

    while time.monotonic() < deadline:
        latest_jobs = fetch_jobs(api_base_url, job_ids)
        for job in latest_jobs.values():
            if job["status"] == "in_progress":
                run_compose_command(["kill", "worker"])
                state.worker_killed = True
                state.worker_killed_at = utc_now_iso()
                state.worker_killed_after_job_id = job["job_id"]
                print(f"  worker killed while job {job['job_id']} was in progress")
                return latest_jobs
        time.sleep(poll_interval)

    raise RuntimeError("No job entered in_progress before kill timeout expired.")


def wait_for_recovery_signal(
    api_base_url: str,
    job_ids: list[str],
    poll_interval: float,
    recovery_timeout: float,
    state: ExperimentState,
) -> dict[str, dict[str, Any]]:
    print("Waiting for the reaper to recover stale work...")
    deadline = time.monotonic() + recovery_timeout
    latest_jobs: dict[str, dict[str, Any]] = {}
    initial_metrics = api_get_json(api_base_url, "/metrics")
    baseline_recovered_stale = int(initial_metrics.get("jobs_recovered_stale", 0))
    baseline_recovered_failed = int(initial_metrics.get("jobs_recovered_failed", 0))

    while time.monotonic() < deadline:
        latest_jobs = fetch_jobs(api_base_url, job_ids)
        metrics = api_get_json(api_base_url, "/metrics")
        recovered_stale = int(metrics.get("jobs_recovered_stale", 0))
        recovered_failed = int(metrics.get("jobs_recovered_failed", 0))
        if recovered_stale > baseline_recovered_stale or recovered_failed > baseline_recovered_failed:
            state.recovery_detected_at = utc_now_iso()
            print(
                "  recovery observed via metrics: "
                f"jobs_recovered_stale={recovered_stale}, jobs_recovered_failed={recovered_failed}"
            )
            return latest_jobs
        time.sleep(poll_interval)

    raise RuntimeError("Stale-job recovery was not observed before recovery timeout expired.")


def wait_for_terminal_jobs(
    api_base_url: str,
    job_ids: list[str],
    poll_interval: float,
    completion_timeout: float,
) -> dict[str, dict[str, Any]]:
    print("Waiting for all jobs to finish after the worker restart...")
    deadline = time.monotonic() + completion_timeout
    latest_jobs: dict[str, dict[str, Any]] = {}

    while time.monotonic() < deadline:
        latest_jobs = fetch_jobs(api_base_url, job_ids)
        if all(job["status"] in {"completed", "failed"} for job in latest_jobs.values()):
            return latest_jobs
        time.sleep(poll_interval)

    raise RuntimeError("Not all jobs reached a terminal state before completion timeout expired.")


def fetch_jobs(api_base_url: str, job_ids: list[str]) -> dict[str, dict[str, Any]]:
    return {job_id: api_get_json(api_base_url, f"/jobs/{job_id}") for job_id in job_ids}


def poll_metrics_forever(
    api_base_url: str,
    interval_seconds: float,
    rows: list[dict[str, Any]],
    stop_event: threading.Event,
) -> None:
    while not stop_event.is_set():
        sampled_at = utc_now_iso()
        row: dict[str, Any] = {"sampled_at": sampled_at}
        try:
            row.update(api_get_json(api_base_url, "/metrics"))
            row["error"] = ""
        except Exception as exc:  # pragma: no cover - best effort
            row["error"] = str(exc)
        rows.append(row)
        stop_event.wait(interval_seconds)


def build_job_rows(latest_jobs: dict[str, dict[str, Any]], job_ids: list[str]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for order, job_id in enumerate(job_ids, start=1):
        job = latest_jobs[job_id]
        rows.append(
            {
                "job_order": order,
                "job_id": job["job_id"],
                "status": job["status"],
                "file_name": job.get("file_name"),
                "file_size_bytes": job.get("file_size_bytes"),
                "created_at": job.get("created_at"),
                "started_at": job.get("processing_started_at"),
                "processing_started_at": job.get("processing_started_at"),
                "completed_at": job.get("completed_at"),
                "error": job.get("error"),
                "queue_wait_s": seconds_between(job.get("created_at"), job.get("processing_started_at")),
                "last_attempt_runtime_s": seconds_between(
                    job.get("processing_started_at"),
                    job.get("completed_at"),
                ),
                "end_to_end_s": seconds_between(job.get("created_at"), job.get("completed_at")),
                "duration_seconds": nested_float(job, "result", "duration_seconds"),
                "simulated_analysis_seconds": nested_float(job, "result", "simulated_analysis_seconds"),
            }
        )
    return rows


def build_summary(
    args: argparse.Namespace,
    state: ExperimentState,
    job_rows: list[dict[str, Any]],
    final_metrics: dict[str, Any],
    result_dir: Path,
    notes: list[str],
) -> dict[str, Any]:
    completed_jobs = [row for row in job_rows if row["status"] == "completed"]
    failed_jobs = [row for row in job_rows if row["status"] == "failed"]
    recovered_stale = int(final_metrics.get("jobs_recovered_stale", 0))
    recovered_failed = int(final_metrics.get("jobs_recovered_failed", 0))
    retried_jobs = recovered_stale + recovered_failed

    return {
        "experiment": "phase_4_crash_recovery",
        "run_at": utc_now_iso(),
        "video_path": str(Path(args.video_path).expanduser().resolve()),
        "burst_size": args.burst_size,
        "worker_killed": state.worker_killed,
        "worker_killed_at": state.worker_killed_at,
        "worker_killed_after_job_id": state.worker_killed_after_job_id,
        "recovery_detected_at": state.recovery_detected_at,
        "worker_restarted_at": state.worker_restarted_at,
        "completed_jobs": len(completed_jobs),
        "failed_jobs": len(failed_jobs),
        "retried_jobs": retried_jobs,
        "max_retry_count_observed": None,
        "all_jobs_terminal": len(completed_jobs) + len(failed_jobs) == len(job_rows),
        "recovery_observed": state.recovery_detected_at is not None,
        "final_metrics": final_metrics,
        "artifacts": {
            "job_results_csv": str(result_dir / "job_results.csv"),
            "metrics_timeseries_csv": str(result_dir / "metrics_timeseries.csv"),
            "summary_json": str(result_dir / "summary.json"),
            "summary_markdown": str(result_dir / "summary.md"),
        },
        "notes": notes,
    }


def write_job_results_csv(path: Path, rows: list[dict[str, Any]]) -> None:
    fieldnames = [
        "job_order",
        "job_id",
        "status",
        "file_name",
        "file_size_bytes",
        "created_at",
        "started_at",
        "processing_started_at",
        "completed_at",
        "error",
        "queue_wait_s",
        "last_attempt_runtime_s",
        "end_to_end_s",
        "duration_seconds",
        "simulated_analysis_seconds",
    ]
    write_csv(path, rows, fieldnames)


def write_metrics_csv(path: Path, rows: list[dict[str, Any]]) -> None:
    fieldnames = [
        "sampled_at",
        "jobs_submitted",
        "jobs_completed",
        "jobs_failed",
        "jobs_rejected",
        "jobs_recovered_stale",
        "jobs_recovered_failed",
        "jobs_poison_pill",
        "pending_queue_depth",
        "processing_queue_depth",
        "error",
    ]
    write_csv(path, rows, fieldnames)


def write_csv(path: Path, rows: list[dict[str, Any]], fieldnames: list[str]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, indent=2), encoding="utf-8")


def write_summary_markdown(path: Path, summary: dict[str, Any]) -> None:
    final_metrics = summary.get("final_metrics", {})
    lines = [
        "# Crash Recovery Experiment Summary",
        "",
        f"- Run at: `{summary['run_at']}`",
        f"- Video: `{summary['video_path']}`",
        f"- Burst size: `{summary['burst_size']}`",
        f"- Worker killed: `{summary['worker_killed']}`",
        f"- Worker killed at: `{summary['worker_killed_at']}`",
        f"- Worker killed after job: `{summary['worker_killed_after_job_id']}`",
        f"- Recovery observed at: `{summary['recovery_detected_at']}`",
        f"- Worker restarted at: `{summary['worker_restarted_at']}`",
        f"- Completed jobs: `{summary['completed_jobs']}`",
        f"- Failed jobs: `{summary['failed_jobs']}`",
        f"- Retried jobs: `{summary['retried_jobs']}`",
        f"- Max retry count observed: `{summary['max_retry_count_observed']}`",
        "",
        "## Final Metrics",
        "",
        f"- jobs_submitted: `{final_metrics.get('jobs_submitted')}`",
        f"- jobs_completed: `{final_metrics.get('jobs_completed')}`",
        f"- jobs_failed: `{final_metrics.get('jobs_failed')}`",
        f"- jobs_recovered_stale: `{final_metrics.get('jobs_recovered_stale')}`",
        f"- jobs_recovered_failed: `{final_metrics.get('jobs_recovered_failed')}`",
        f"- jobs_poison_pill: `{final_metrics.get('jobs_poison_pill')}`",
        f"- pending_queue_depth: `{final_metrics.get('pending_queue_depth')}`",
        f"- processing_queue_depth: `{final_metrics.get('processing_queue_depth')}`",
        "",
        "## Artifact Files",
        "",
        f"- `{summary['artifacts']['job_results_csv']}`",
        f"- `{summary['artifacts']['metrics_timeseries_csv']}`",
        f"- `{summary['artifacts']['summary_json']}`",
    ]

    notes = summary.get("notes") or []
    if notes:
        lines.extend(["", "## Notes", ""])
        lines.extend([f"- {note}" for note in notes])

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def create_result_dir() -> Path:
    run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    result_dir = RESULTS_ROOT / run_id
    result_dir.mkdir(parents=True, exist_ok=True)
    return result_dir


def api_get_json(api_base_url: str, path: str) -> dict[str, Any]:
    url = f"{api_base_url.rstrip('/')}{path}"
    try:
        with request.urlopen(url, timeout=10) as response:
            return json.loads(response.read().decode("utf-8"))
    except error.HTTPError as exc:
        body = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"GET {url} failed with HTTP {exc.code}: {body}") from exc


def run_compose_command(args: list[str]) -> subprocess.CompletedProcess[str]:
    cmd = ["docker", "compose", *args]
    return subprocess.run(
        cmd,
        cwd=PROJECT_ROOT,
        check=True,
        capture_output=True,
        text=True,
    )


def restart_worker() -> None:
    run_compose_command(["start", "worker"])


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
