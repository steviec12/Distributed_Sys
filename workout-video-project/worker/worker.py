import json
import logging
import os
import subprocess
import time
from datetime import datetime, timezone

import redis


STATUS_IN_PROGRESS = "in_progress"
STATUS_COMPLETED = "completed"
STATUS_FAILED = "failed"

STATS_JOBS_COMPLETED = "stats:jobs_completed"
STATS_JOBS_FAILED = "stats:jobs_failed"


def get_env(name: str, default: str) -> str:
    value = os.getenv(name)
    return value if value else default


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def probe_duration_seconds(file_path: str) -> float:
    command = [
        "ffprobe",
        "-v",
        "error",
        "-show_entries",
        "format=duration",
        "-of",
        "json",
        file_path,
    ]
    completed = subprocess.run(command, capture_output=True, text=True, check=True)
    payload = json.loads(completed.stdout or "{}")
    duration_text = payload.get("format", {}).get("duration")
    if duration_text is None:
        raise ValueError("ffprobe did not return duration")
    return round(float(duration_text), 3)


def simulated_analysis_seconds(duration_seconds: float) -> float:
    raw_seconds = duration_seconds * 0.2
    bounded_seconds = min(max(raw_seconds, 1.0), 8.0)
    return round(bounded_seconds, 2)


def process_job(client: redis.Redis, job_id: str) -> None:
    job_key = f"job:{job_id}"
    job = client.hgetall(job_key)
    if not job:
        logging.error("job_missing job_id=%s", job_id)
        return

    client.hset(
        job_key,
        mapping={
            "status": STATUS_IN_PROGRESS,
            "started_at": utc_now(),
            "error": "",
        },
    )

    try:
        duration_seconds = probe_duration_seconds(job["file_path"])
        simulated_seconds = simulated_analysis_seconds(duration_seconds)
        logging.info(
            "job_processing job_id=%s duration_seconds=%.3f simulated_analysis_seconds=%.2f",
            job_id,
            duration_seconds,
            simulated_seconds,
        )
        time.sleep(simulated_seconds)

        result = {
            "duration_seconds": duration_seconds,
            "simulated_analysis_seconds": simulated_seconds,
        }

        client.hset(
            job_key,
            mapping={
                "status": STATUS_COMPLETED,
                "completed_at": utc_now(),
                "error": "",
                "result_json": json.dumps(result),
            },
        )
        client.incr(STATS_JOBS_COMPLETED)
        logging.info("job_completed job_id=%s", job_id)
    except Exception as exc:  # pylint: disable=broad-except
        client.hset(
            job_key,
            mapping={
                "status": STATUS_FAILED,
                "completed_at": utc_now(),
                "error": str(exc),
            },
        )
        client.incr(STATS_JOBS_FAILED)
        logging.exception("job_failed job_id=%s", job_id)


def main() -> None:
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(message)s",
    )

    redis_addr = get_env("REDIS_ADDR", "localhost:6379")
    redis_password = get_env("REDIS_PASSWORD", "")
    redis_db = int(get_env("REDIS_DB", "0"))
    queue_key = get_env("QUEUE_KEY", "queue:pending")

    client = redis.Redis(
        host=redis_addr.split(":", 1)[0],
        port=int(redis_addr.split(":", 1)[1]),
        password=redis_password or None,
        db=redis_db,
        decode_responses=True,
    )

    logging.info("worker_started queue=%s redis_addr=%s", queue_key, redis_addr)
    while True:
        _, job_id = client.blpop(queue_key, timeout=0)
        logging.info("job_dequeued job_id=%s", job_id)
        process_job(client, job_id)


if __name__ == "__main__":
    main()
