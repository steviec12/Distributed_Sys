#!/usr/bin/env bash
set -euo pipefail

source "/Users/stevi/Documents/web-service-gin/workout-video-project/scripts/_validation_common.sh"

assert_service_env_equals() {
  local service_name="$1"
  local env_key="$2"
  local expected="$3"
  local actual
  cd "${PROJECT_ROOT}"
  actual="$(docker compose exec -T "${service_name}" printenv "${env_key}" | tr -d '\r')"
  if [[ "${actual}" != "${expected}" ]]; then
    echo "Expected ${service_name} ${env_key}=${expected}, got ${actual}" >&2
    return 1
  fi
}

echo "Step 7 core validation starting."

run_standard_go_validations
wait_for_api

for service_name in api worker reaper; do
  if ! service_is_running "${service_name}"; then
    echo "Service ${service_name} is not running." >&2
    exit 1
  fi
done

echo ""
echo "==> Verifying running services are explicitly in local deployment mode"
assert_service_env_equals api DEPLOYMENT_MODE local
assert_service_env_equals worker DEPLOYMENT_MODE local
assert_service_env_equals reaper DEPLOYMENT_MODE local

echo ""
echo "Step 7 core validation finished."
