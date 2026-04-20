#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="/Users/stevi/Documents/web-service-gin/workout-video-project"
VIDEO_PATH="${1:-}"
API_URL="${API_URL:-http://localhost:8080}"

if [[ -z "${VIDEO_PATH}" ]]; then
  echo "Usage: $0 /absolute/path/to/video.mp4"
  exit 1
fi

if [[ ! -f "${VIDEO_PATH}" ]]; then
  echo "Video file not found: ${VIDEO_PATH}"
  exit 1
fi

run_go_validation() {
  local service_dir="$1"
  echo ""
  echo "==> Validating ${service_dir}"
  cd "${PROJECT_ROOT}/${service_dir}"
  go mod tidy
  go test ./...
}

wait_for_api() {
  echo ""
  echo "==> Waiting for API health"
  for _ in {1..60}; do
    if curl -fsS "${API_URL}/health" >/dev/null 2>&1; then
      echo "API is healthy."
      return 0
    fi
    sleep 2
  done

  echo "API did not become healthy in time."
  return 1
}

submit_job() {
  echo "" >&2
  echo "==> Creating upload session, uploading parts, and finalizing" >&2
  python3 "${PROJECT_ROOT}/analysis/upload_client.py" \
    --api-base-url "${API_URL}" \
    --video-path "${VIDEO_PATH}" \
    --output json
}

extract_job_id() {
  local payload="$1"
  printf '%s' "${payload}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job_id"])'
}

poll_job() {
  local job_id="$1"
  echo "" >&2
  echo "==> Polling job ${job_id}" >&2
  for _ in {1..120}; do
    local payload
    payload="$(curl -fsS "${API_URL}/jobs/${job_id}")"
    local status
    status="$(printf '%s' "${payload}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')"
    if [[ "${status}" == "completed" || "${status}" == "failed" ]]; then
      echo "${payload}"
      return 0
    fi
    sleep 1
  done

  echo "Job ${job_id} did not reach a terminal state in time." >&2
  return 1
}

query_dynamo_item() {
  local job_id="$1"
  if ! command -v aws >/dev/null 2>&1; then
    echo ""
    echo "==> Skipping DynamoDB Local query because aws CLI is not installed"
    return 0
  fi

  echo ""
  echo "==> Querying DynamoDB Local for durable record"
  aws dynamodb get-item \
    --no-cli-pager \
    --endpoint-url http://localhost:8000 \
    --region us-west-2 \
    --table-name workout_jobs \
    --key "{\"job_id\":{\"S\":\"${job_id}\"}}"
}

run_crash_recovery() {
  echo ""
  echo "==> Running crash-recovery regression through upload-session API"
  cd "${PROJECT_ROOT}"
  python3 analysis/run_crash_recovery_experiment.py \
    --video-path "${VIDEO_PATH}" \
    --burst-size 20
}

echo "Step 3 validation starting."

run_go_validation "api"
run_go_validation "go-worker"
run_go_validation "reaper"

wait_for_api

upload_payload="$(submit_job)"
echo "${upload_payload}"

job_id="$(extract_job_id "${upload_payload}")"
final_job_payload="$(poll_job "${job_id}")"

echo ""
echo "==> Final job response"
echo "${final_job_payload}"

query_dynamo_item "${job_id}"
run_crash_recovery

echo ""
echo "Step 3 validation finished."
