#!/usr/bin/env bash
set -euo pipefail

VIDEO_PATH="${1:-}"
MAX_RETRIES="${MAX_RETRIES:-3}"
source "/Users/stevi/Documents/web-service-gin/workout-video-project/scripts/_validation_common.sh"

require_video_path "${VIDEO_PATH}"
require_aws_cli

worker_was_stopped=0

cleanup() {
  if [[ "${worker_was_stopped}" == "1" ]]; then
    ensure_worker_running
  fi
}

trap cleanup EXIT

seed_timestamp() {
  python3 - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) - timedelta(minutes=2)).strftime("%Y-%m-%dT%H:%M:%S.000Z"))
PY
}

seed_stale_attempt() {
  local job_id="$1"
  local old_timestamp="$2"
  cd "${PROJECT_ROOT}"
  docker compose exec -T redis redis-cli LREM queue:pending 1 "${job_id}" >/dev/null
  docker compose exec -T redis redis-cli HSET "job:${job_id}" \
    status in_progress \
    processing_started_at "${old_timestamp}" \
    last_heartbeat_at "${old_timestamp}" \
    started_at "${old_timestamp}" \
    worker_id seeded-worker \
    retry_count 0 \
    failure_type none \
    error "" \
    result_json "" >/dev/null
  docker compose exec -T redis redis-cli RPUSH queue:processing "${job_id}" >/dev/null

  aws dynamodb update-item \
    --no-cli-pager \
    --endpoint-url http://localhost:8000 \
    --region us-west-2 \
    --table-name workout_jobs \
    --key "{\"job_id\":{\"S\":\"${job_id}\"}}" \
    --update-expression "SET #status = :status, processing_started_at = :processing_started_at, started_at = :started_at, worker_id = :worker_id, retry_count = :retry_count, failure_type = :failure_type REMOVE completed_at, #error, result_json" \
    --expression-attribute-names '{"#status":"status","#error":"error"}' \
    --expression-attribute-values "{\":status\":{\"S\":\"in_progress\"},\":processing_started_at\":{\"S\":\"${old_timestamp}\"},\":started_at\":{\"S\":\"${old_timestamp}\"},\":worker_id\":{\"S\":\"seeded-worker\"},\":retry_count\":{\"N\":\"0\"},\":failure_type\":{\"S\":\"none\"}}" >/dev/null
}

seed_retryable_failed_max_attempt() {
  local job_id="$1"
  cd "${PROJECT_ROOT}"
  docker compose exec -T redis redis-cli LREM queue:pending 1 "${job_id}" >/dev/null
  docker compose exec -T redis redis-cli LREM queue:processing 1 "${job_id}" >/dev/null
  docker compose exec -T redis redis-cli HSET "job:${job_id}" \
    status failed \
    retry_count "${MAX_RETRIES}" \
    failure_type processing_error \
    error "seeded processing failure" \
    completed_at "" \
    result_json "" >/dev/null
  docker compose exec -T redis redis-cli SADD set:retryable_failed "${job_id}" >/dev/null

  aws dynamodb update-item \
    --no-cli-pager \
    --endpoint-url http://localhost:8000 \
    --region us-west-2 \
    --table-name workout_jobs \
    --key "{\"job_id\":{\"S\":\"${job_id}\"}}" \
    --update-expression "SET #status = :status, retry_count = :retry_count, failure_type = :failure_type, #error = :error REMOVE completed_at, worker_id, processing_started_at, result_json" \
    --expression-attribute-names '{"#status":"status","#error":"error"}' \
    --expression-attribute-values "{\":status\":{\"S\":\"failed\"},\":retry_count\":{\"N\":\"${MAX_RETRIES}\"},\":failure_type\":{\"S\":\"processing_error\"},\":error\":{\"S\":\"seeded processing failure\"}}" >/dev/null
}

echo "Step 6 core validation starting."

run_standard_go_validations
wait_for_api

if ! service_is_running "reaper"; then
  echo "Reaper service is not running. Step 6 validation needs the reaper active."
  exit 1
fi

if worker_is_running; then
  stop_worker
  worker_was_stopped=1
else
  echo ""
  echo "==> Worker is already stopped; leaving it stopped after validation"
fi

echo ""
echo "==> Step 6A: proving stale recovery is written durably to DynamoDB"
stale_upload_payload="$(submit_job "${VIDEO_PATH}")"
echo "${stale_upload_payload}"
stale_job_id="$(extract_job_id "${stale_upload_payload}")"
old_timestamp="$(seed_timestamp)"
seed_stale_attempt "${stale_job_id}" "${old_timestamp}"

stale_job_payload="$(wait_for_job_status "${stale_job_id}" "queued" 30)"
assert_job_status "${stale_job_payload}" "queued"

echo ""
echo "==> Stale recovery job response"
echo "${stale_job_payload}"

query_dynamo_item "${stale_job_id}"
query_redis_item "${stale_job_id}"
assert_dynamo_field_equals "${stale_job_id}" "status" "queued"
assert_dynamo_field_equals "${stale_job_id}" "retry_count" "1"
assert_dynamo_field_equals "${stale_job_id}" "failure_type" "stale_timeout"
assert_dynamo_field_equals "${stale_job_id}" "error" "requeued after stale timeout"
assert_dynamo_field_missing "${stale_job_id}" "completed_at"
assert_dynamo_field_missing "${stale_job_id}" "processing_started_at"
assert_dynamo_field_missing "${stale_job_id}" "worker_id"

echo ""
echo "==> Step 6B: proving poison-pill recovery is written durably to DynamoDB"
poison_upload_payload="$(submit_job "${VIDEO_PATH}")"
echo "${poison_upload_payload}"
poison_job_id="$(extract_job_id "${poison_upload_payload}")"
seed_retryable_failed_max_attempt "${poison_job_id}"

wait_for_dynamo_field_equals "${poison_job_id}" "failure_type" "poison_pill" 30
poison_job_payload="$(wait_for_job_status "${poison_job_id}" "failed" 30)"
assert_job_status "${poison_job_payload}" "failed"
assert_job_error_contains "${poison_job_payload}" "retry limit exceeded"

echo ""
echo "==> Poison-pill job response"
echo "${poison_job_payload}"

query_dynamo_item "${poison_job_id}"
query_redis_item "${poison_job_id}"
assert_dynamo_field_equals "${poison_job_id}" "status" "failed"
assert_dynamo_field_equals "${poison_job_id}" "retry_count" "${MAX_RETRIES}"
assert_dynamo_field_equals "${poison_job_id}" "failure_type" "poison_pill"
assert_dynamo_field_equals "${poison_job_id}" "error" "retry limit exceeded after processing error"
assert_dynamo_field_present "${poison_job_id}" "completed_at"

echo ""
echo "Step 6 core validation finished."
