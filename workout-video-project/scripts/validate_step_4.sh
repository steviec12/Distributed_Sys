#!/usr/bin/env bash
set -euo pipefail

VIDEO_PATH="${1:-}"
source "/Users/stevi/Documents/web-service-gin/workout-video-project/scripts/_validation_common.sh"

require_video_path "${VIDEO_PATH}"

echo "Step 4 core validation starting."

run_standard_go_validations

wait_for_api

upload_payload="$(submit_job "${VIDEO_PATH}")"
echo "${upload_payload}"

job_id="$(extract_job_id "${upload_payload}")"
final_job_payload="$(poll_job "${job_id}")"
assert_job_completed "${final_job_payload}"

echo ""
echo "==> Final job response"
echo "${final_job_payload}"

query_dynamo_item "${job_id}"
query_redis_item "${job_id}"
assert_redis_field_present "${job_id}" "s3_key"
assert_redis_field_absent "${job_id}" "file_path"

echo ""
echo "Step 4 core validation finished."
