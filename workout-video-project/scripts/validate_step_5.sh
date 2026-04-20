#!/usr/bin/env bash
set -euo pipefail

VIDEO_PATH="${1:-}"
source "/Users/stevi/Documents/web-service-gin/workout-video-project/scripts/_validation_common.sh"

require_video_path "${VIDEO_PATH}"
worker_was_stopped=0

cleanup() {
  if [[ "${worker_was_stopped}" == "1" ]]; then
    ensure_worker_running
  fi
}

trap cleanup EXIT

echo "Step 5 core validation starting."

run_standard_go_validations

wait_for_api
stop_worker
worker_was_stopped=1

upload_payload="$(submit_job "${VIDEO_PATH}")"
echo "${upload_payload}"

job_id="$(extract_job_id "${upload_payload}")"

query_dynamo_item "${job_id}"
query_redis_item "${job_id}"
corrupt_redis_metadata "${job_id}"
assert_redis_field_absent "${job_id}" "s3_key"
assert_redis_field_absent "${job_id}" "file_name"
assert_redis_field_absent "${job_id}" "content_type"
query_redis_item "${job_id}"

start_worker

final_job_payload="$(poll_job "${job_id}")"
assert_job_completed "${final_job_payload}"

echo ""
echo "==> Final job response"
echo "${final_job_payload}"

query_dynamo_item "${job_id}"
query_redis_item "${job_id}"

echo ""
echo "Step 5 core validation finished."
