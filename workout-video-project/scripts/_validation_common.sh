#!/usr/bin/env bash

PROJECT_ROOT="${PROJECT_ROOT:-/Users/stevi/Documents/web-service-gin/workout-video-project}"
API_URL="${API_URL:-http://localhost:8080}"

require_video_path() {
  local video_path="$1"
  if [[ -z "${video_path}" ]]; then
    echo "Usage: $0 /absolute/path/to/video.mp4"
    exit 1
  fi

  if [[ ! -f "${video_path}" ]]; then
    echo "Video file not found: ${video_path}"
    exit 1
  fi
}

require_aws_cli() {
  if ! command -v aws >/dev/null 2>&1; then
    echo "This validation requires the aws CLI to query DynamoDB Local."
    exit 1
  fi
}

run_go_validation() {
  local service_dir="$1"
  echo ""
  echo "==> Validating ${service_dir}"
  cd "${PROJECT_ROOT}/${service_dir}"
  go mod tidy
  go test ./...
}

run_standard_go_validations() {
  run_go_validation "api"
  run_go_validation "go-worker"
  run_go_validation "reaper"
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
  local video_path="$1"
  echo "" >&2
  echo "==> Creating upload session, uploading parts, and finalizing" >&2
  python3 "${PROJECT_ROOT}/analysis/upload_client.py" \
    --api-base-url "${API_URL}" \
    --video-path "${video_path}" \
    --output json
}

extract_job_id() {
  local payload="$1"
  printf '%s' "${payload}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["job_id"])'
}

extract_job_status() {
  local payload="$1"
  printf '%s' "${payload}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])'
}

get_job_payload() {
  local job_id="$1"
  curl -fsS "${API_URL}/jobs/${job_id}"
}

assert_job_completed() {
  local payload="$1"
  local status
  status="$(extract_job_status "${payload}")"
  if [[ "${status}" != "completed" ]]; then
    echo "Expected job to complete, but final status was: ${status}" >&2
    echo "${payload}" >&2
    return 1
  fi
}

assert_job_status() {
  local payload="$1"
  local expected="$2"
  local status
  status="$(extract_job_status "${payload}")"
  if [[ "${status}" != "${expected}" ]]; then
    echo "Expected job status ${expected}, but got ${status}." >&2
    echo "${payload}" >&2
    return 1
  fi
}

assert_job_error_contains() {
  local payload="$1"
  local expected_fragment="$2"
  local actual
  actual="$(printf '%s' "${payload}" | python3 -c 'import json,sys; value=json.load(sys.stdin).get("error"); print("" if value is None else value)')"
  if [[ "${actual}" != *"${expected_fragment}"* ]]; then
    echo "Expected job error to contain '${expected_fragment}', but got '${actual}'." >&2
    echo "${payload}" >&2
    return 1
  fi
}

poll_job() {
  local job_id="$1"
  echo "" >&2
  echo "==> Polling job ${job_id}" >&2
  for _ in {1..120}; do
    local payload
    payload="$(curl -fsS "${API_URL}/jobs/${job_id}")"
    local status
    status="$(extract_job_status "${payload}")"
    if [[ "${status}" == "completed" || "${status}" == "failed" ]]; then
      echo "${payload}"
      return 0
    fi
    sleep 1
  done

  echo "Job ${job_id} did not reach a terminal state in time." >&2
  return 1
}

wait_for_job_status() {
  local job_id="$1"
  local expected_status="$2"
  local attempts="${3:-120}"
  echo "" >&2
  echo "==> Waiting for job ${job_id} to reach status ${expected_status}" >&2
  for ((i = 0; i < attempts; i++)); do
    local payload
    payload="$(get_job_payload "${job_id}")"
    local status
    status="$(extract_job_status "${payload}")"
    if [[ "${status}" == "${expected_status}" ]]; then
      echo "${payload}"
      return 0
    fi
    sleep 1
  done

  echo "Job ${job_id} did not reach status ${expected_status} in time." >&2
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

get_dynamo_item_json() {
  local job_id="$1"
  aws dynamodb get-item \
    --no-cli-pager \
    --endpoint-url http://localhost:8000 \
    --region us-west-2 \
    --table-name workout_jobs \
    --key "{\"job_id\":{\"S\":\"${job_id}\"}}"
}

assert_dynamo_field_equals() {
  local job_id="$1"
  local field="$2"
  local expected="$3"
  local item_json
  item_json="$(get_dynamo_item_json "${job_id}")"
  ITEM_JSON="${item_json}" python3 - "${field}" "${expected}" <<'PY'
import json
import os
import sys

field = sys.argv[1]
expected = sys.argv[2]
item = json.loads(os.environ["ITEM_JSON"]).get("Item", {})
value = item.get(field)
if value is None:
    print(f"missing field {field}", file=sys.stderr)
    sys.exit(1)
if "S" in value:
    actual = value["S"]
elif "N" in value:
    actual = value["N"]
else:
    print(f"unsupported DynamoDB attribute encoding for {field}: {value}", file=sys.stderr)
    sys.exit(1)
if actual != expected:
    print(f"expected {field}={expected}, got {actual}", file=sys.stderr)
    sys.exit(1)
PY
}

assert_dynamo_field_missing() {
  local job_id="$1"
  local field="$2"
  local item_json
  item_json="$(get_dynamo_item_json "${job_id}")"
  ITEM_JSON="${item_json}" python3 - "${field}" <<'PY'
import json
import os
import sys

field = sys.argv[1]
item = json.loads(os.environ["ITEM_JSON"]).get("Item", {})
if field in item:
    print(f"expected field {field} to be missing, but it was present", file=sys.stderr)
    sys.exit(1)
PY
}

assert_dynamo_field_present() {
  local job_id="$1"
  local field="$2"
  local item_json
  item_json="$(get_dynamo_item_json "${job_id}")"
  ITEM_JSON="${item_json}" python3 - "${field}" <<'PY'
import json
import os
import sys

field = sys.argv[1]
item = json.loads(os.environ["ITEM_JSON"]).get("Item", {})
if field not in item:
    print(f"expected field {field} to be present, but it was missing", file=sys.stderr)
    sys.exit(1)
PY
}

wait_for_dynamo_field_equals() {
  local job_id="$1"
  local field="$2"
  local expected="$3"
  local attempts="${4:-60}"
  echo "" >&2
  echo "==> Waiting for DynamoDB field ${field} on ${job_id} to become ${expected}" >&2
  for ((i = 0; i < attempts; i++)); do
    if assert_dynamo_field_equals "${job_id}" "${field}" "${expected}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "DynamoDB field ${field} for job ${job_id} did not become ${expected} in time." >&2
  return 1
}

query_redis_item() {
  local job_id="$1"
  echo ""
  echo "==> Querying Redis live job hash"
  cd "${PROJECT_ROOT}"
  docker compose exec -T redis redis-cli HGETALL "job:${job_id}"
}

redis_field_exists() {
  local job_id="$1"
  local field="$2"
  cd "${PROJECT_ROOT}"
  docker compose exec -T redis redis-cli HEXISTS "job:${job_id}" "${field}" | tr -d '\r'
}

assert_redis_field_present() {
  local job_id="$1"
  local field="$2"
  local exists
  exists="$(redis_field_exists "${job_id}" "${field}")"
  if [[ "${exists}" != "1" ]]; then
    echo "Expected Redis hash job:${job_id} to contain field ${field}, but it was missing." >&2
    return 1
  fi
}

assert_redis_field_absent() {
  local job_id="$1"
  local field="$2"
  local exists
  exists="$(redis_field_exists "${job_id}" "${field}")"
  if [[ "${exists}" != "0" ]]; then
    echo "Expected Redis hash job:${job_id} to omit field ${field}, but it was still present." >&2
    return 1
  fi
}

worker_is_running() {
  cd "${PROJECT_ROOT}"
  docker compose ps --status running --services | grep -qx "worker"
}

service_is_running() {
  local service_name="$1"
  cd "${PROJECT_ROOT}"
  docker compose ps --status running --services | grep -qx "${service_name}"
}

stop_worker() {
  echo ""
  echo "==> Stopping worker so the queue can be prepared first"
  cd "${PROJECT_ROOT}"
  docker compose stop worker
}

start_worker() {
  echo ""
  echo "==> Starting worker"
  cd "${PROJECT_ROOT}"
  docker compose start worker
}

ensure_worker_running() {
  if ! worker_is_running; then
    start_worker
  fi
}

corrupt_redis_metadata() {
  local job_id="$1"
  echo ""
  echo "==> Corrupting Redis job metadata to prove the worker reads DynamoDB"
  cd "${PROJECT_ROOT}"
  docker compose exec -T redis redis-cli HDEL "job:${job_id}" s3_key file_name content_type
}

run_crash_recovery_regression() {
  local video_path="$1"
  echo ""
  echo "==> Running crash-recovery regression"
  cd "${PROJECT_ROOT}"
  python3 analysis/run_crash_recovery_experiment.py \
    --video-path "${video_path}" \
    --burst-size 20
}
