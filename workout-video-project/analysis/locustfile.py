"""
Locust load driver for the workout video upload pipeline.

Each simulated user picks a random video from a local directory,
runs the full 3-step upload flow (create session → upload parts to S3 → finalize),
and records the job_id for later result collection.

Usage:
    locust -f analysis/locustfile.py \
        --host http://<ALB_URL> \
        --users 100 --spawn-rate 100 \
        --run-time 120s --headless \
        --csv analysis/results/locust

Environment variables:
    VIDEO_DIR    Directory containing test video files (required)
    JOB_ID_FILE  Path to write submitted job IDs (default: analysis/results/job_ids.txt)
"""

from __future__ import annotations

import json
import mimetypes
import os
import random
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

from locust import HttpUser, between, events, task
from locust.exception import StopUser


VIDEO_DIR = os.environ.get("VIDEO_DIR", "")
JOB_ID_FILE = os.environ.get("JOB_ID_FILE", "analysis/results/job_ids.txt")

_video_files: list[Path] = []
_job_id_lock = threading.Lock()
_job_id_handle = None


def _discover_videos() -> list[Path]:
    if not VIDEO_DIR:
        raise RuntimeError("VIDEO_DIR environment variable is required")
    video_dir = Path(VIDEO_DIR)
    if not video_dir.is_dir():
        raise RuntimeError(f"VIDEO_DIR is not a directory: {video_dir}")

    extensions = {".mp4", ".mov", ".avi", ".mkv", ".webm"}
    videos = sorted(
        p for p in video_dir.iterdir()
        if p.is_file() and p.suffix.lower() in extensions
    )
    if not videos:
        raise RuntimeError(f"No video files found in {video_dir}")
    return videos


@events.init.add_listener
def on_init(environment, **kwargs):
    global _video_files, _job_id_handle
    _video_files = _discover_videos()
    print(f"Discovered {len(_video_files)} test videos in {VIDEO_DIR}")
    for v in _video_files:
        print(f"  {v.name} ({v.stat().st_size} bytes)")

    job_id_path = Path(JOB_ID_FILE)
    job_id_path.parent.mkdir(parents=True, exist_ok=True)
    _job_id_handle = open(job_id_path, "w")


@events.quitting.add_listener
def on_quitting(environment, **kwargs):
    global _job_id_handle
    if _job_id_handle:
        _job_id_handle.close()
        _job_id_handle = None


def _record_job_id(job_id: str) -> None:
    with _job_id_lock:
        if _job_id_handle:
            _job_id_handle.write(job_id + "\n")
            _job_id_handle.flush()


class VideoUploadUser(HttpUser):
    wait_time = between(0, 0)

    @task
    def upload_video(self):
        video_path = random.choice(_video_files)

        job_id = self._create_and_upload(video_path)
        if job_id:
            _record_job_id(job_id)

        raise StopUser()

    def _create_and_upload(self, video_path: Path) -> str | None:
        content_type = mimetypes.guess_type(video_path.name)[0] or "application/octet-stream"
        file_size = video_path.stat().st_size

        with self.client.post(
            "/uploads",
            json={
                "file_name": video_path.name,
                "file_size_bytes": file_size,
                "content_type": content_type,
            },
            name="POST /uploads",
            catch_response=True,
        ) as response:
            if response.status_code != 202:
                response.failure(f"create session failed: {response.status_code}")
                return None
            upload_data = response.json()
            response.success()

        job_id = upload_data["job_id"]
        part_size = int(upload_data["part_size"])
        parts = upload_data["parts"]

        completed_parts = []
        with video_path.open("rb") as source:
            for part in parts:
                part_number = int(part["part_number"])
                upload_url = part["upload_url"]
                offset = (part_number - 1) * part_size

                source.seek(offset)
                chunk = source.read(part_size)
                if not chunk:
                    return None

                start_time = time.time()
                try:
                    req = urllib.request.Request(
                        upload_url,
                        data=chunk,
                        method="PUT",
                        headers={"Content-Length": str(len(chunk))},
                    )
                    with urllib.request.urlopen(req, timeout=120) as resp:
                        etag = resp.headers.get("ETag")

                    elapsed = (time.time() - start_time) * 1000
                    events.request.fire(
                        request_type="PUT",
                        name="PUT S3 part",
                        response_time=elapsed,
                        response_length=0,
                        exception=None,
                        context={},
                    )
                except Exception as exc:
                    elapsed = (time.time() - start_time) * 1000
                    events.request.fire(
                        request_type="PUT",
                        name="PUT S3 part",
                        response_time=elapsed,
                        response_length=0,
                        exception=exc,
                        context={},
                    )
                    return None

                if not etag:
                    return None

                completed_parts.append({"part_number": part_number, "etag": etag})

        with self.client.post(
            f"/jobs/{job_id}/finalize",
            json={"parts": completed_parts},
            name="POST /jobs/:id/finalize",
            catch_response=True,
        ) as response:
            if response.status_code != 202:
                response.failure(f"finalize failed: {response.status_code}")
                return None
            response.success()

        return job_id


