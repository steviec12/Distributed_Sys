#!/usr/bin/env bash
set -euo pipefail

VIDEO_PATH="${1:-}"

source "/Users/stevi/Documents/web-service-gin/workout-video-project/scripts/_validation_common.sh"

require_video_path "${VIDEO_PATH}"

echo "Crash-recovery regression starting."

wait_for_api
run_crash_recovery_regression "${VIDEO_PATH}"

echo ""
echo "Crash-recovery regression finished."
