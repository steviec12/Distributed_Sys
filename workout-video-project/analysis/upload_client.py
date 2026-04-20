from __future__ import annotations

import argparse
import json
import mimetypes
from pathlib import Path
from typing import Any
from urllib import error, request


DEFAULT_API_BASE_URL = "http://localhost:8080"


def api_post_json(api_base_url: str, path: str, payload: dict[str, Any]) -> dict[str, Any]:
    url = f"{api_base_url.rstrip('/')}{path}"
    body = json.dumps(payload).encode("utf-8")
    req = request.Request(
        url,
        data=body,
        method="POST",
        headers={"Content-Type": "application/json"},
    )

    try:
        with request.urlopen(req, timeout=30) as response:
            return json.loads(response.read().decode("utf-8"))
    except error.HTTPError as exc:
        body_text = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"POST {url} failed with HTTP {exc.code}: {body_text}") from exc


def create_upload_session(api_base_url: str, video_path: Path) -> dict[str, Any]:
    content_type = mimetypes.guess_type(video_path.name)[0] or "application/octet-stream"
    return api_post_json(
        api_base_url,
        "/uploads",
        {
            "file_name": video_path.name,
            "file_size_bytes": video_path.stat().st_size,
            "content_type": content_type,
        },
    )


def upload_parts(video_path: Path, upload_response: dict[str, Any]) -> list[dict[str, Any]]:
    part_size = int(upload_response["part_size"])
    completed_parts: list[dict[str, Any]] = []

    with video_path.open("rb") as source:
        for part in upload_response["parts"]:
            part_number = int(part["part_number"])
            upload_url = part["upload_url"]
            offset = (part_number - 1) * part_size

            source.seek(offset)
            chunk = source.read(part_size)
            if not chunk:
                raise RuntimeError(f"no bytes available for part {part_number}")

            req = request.Request(
                upload_url,
                data=chunk,
                method="PUT",
                headers={"Content-Length": str(len(chunk))},
            )
            try:
                with request.urlopen(req, timeout=120) as response:
                    etag = response.headers.get("ETag")
            except error.HTTPError as exc:
                body_text = exc.read().decode("utf-8", errors="replace")
                raise RuntimeError(f"PUT part {part_number} failed with HTTP {exc.code}: {body_text}") from exc

            if not etag:
                raise RuntimeError(f"S3 upload part {part_number} did not return an ETag")

            completed_parts.append({"part_number": part_number, "etag": etag})

    return completed_parts


def finalize_upload(api_base_url: str, job_id: str, completed_parts: list[dict[str, Any]]) -> dict[str, Any]:
    return api_post_json(
        api_base_url,
        f"/jobs/{job_id}/finalize",
        {"parts": completed_parts},
    )


def submit_video_job(api_base_url: str, video_path: Path) -> dict[str, Any]:
    upload_response = create_upload_session(api_base_url, video_path)
    completed_parts = upload_parts(video_path, upload_response)
    finalize_response = finalize_upload(api_base_url, upload_response["job_id"], completed_parts)
    return {
        "job_id": upload_response["job_id"],
        "upload": upload_response,
        "finalize": finalize_response,
        "parts": completed_parts,
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Upload a video through the Step 3 upload-session API.")
    parser.add_argument("--video-path", required=True, help="Absolute path to the local video file.")
    parser.add_argument("--api-base-url", default=DEFAULT_API_BASE_URL, help="API base URL.")
    parser.add_argument(
        "--output",
        choices=["json", "job_id"],
        default="json",
        help="Output format.",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    video_path = Path(args.video_path).expanduser().resolve()
    if not video_path.exists():
        raise SystemExit(f"Video file does not exist: {video_path}")

    result = submit_video_job(args.api_base_url, video_path)
    if args.output == "job_id":
        print(result["job_id"])
        return

    print(json.dumps(result))


if __name__ == "__main__":
    main()
