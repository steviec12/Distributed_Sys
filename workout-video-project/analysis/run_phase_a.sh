#!/usr/bin/env bash
set -euo pipefail

# Phase A: Capacity Profiling — single run for a given worker count.
#
# Usage:
#   bash analysis/run_phase_a.sh \
#     --api-url http://<ALB_URL> \
#     --video-dir /Users/stevi/Documents/cs6650_final \
#     --workers 4 \
#     --users 100 \
#     --redis-host workout-video-redis.odjz60.0001.usw2.cache.amazonaws.com
#
# Prerequisites:
#   pip3 install locust

API_URL=""
VIDEO_DIR=""
WORKERS=""
USERS="100"
REDIS_HOST=""
REDIS_PORT="6379"
ECS_CLUSTER="workout-video-cluster"
WORKER_SERVICE="workout-video-worker"
AWS_REGION="us-west-2"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-url) API_URL="$2"; shift 2 ;;
    --video-dir) VIDEO_DIR="$2"; shift 2 ;;
    --workers) WORKERS="$2"; shift 2 ;;
    --users) USERS="$2"; shift 2 ;;
    --redis-host) REDIS_HOST="$2"; shift 2 ;;
    --redis-port) REDIS_PORT="$2"; shift 2 ;;
    --cluster) ECS_CLUSTER="$2"; shift 2 ;;
    --worker-service) WORKER_SERVICE="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$API_URL" || -z "$VIDEO_DIR" || -z "$WORKERS" ]]; then
  echo "Required: --api-url, --video-dir, --workers"
  exit 1
fi

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS_DIR="${PROJECT_ROOT}/analysis/results"
JOB_ID_FILE="${RESULTS_DIR}/job_ids.txt"
METRICS_FILE="${RESULTS_DIR}/metrics_live.csv"
LABEL="${WORKERS}_workers"

echo "================================================"
echo "Phase A: Capacity Profiling — ${WORKERS} workers"
echo "================================================"
echo "API:       ${API_URL}"
echo "Video dir: ${VIDEO_DIR}"
echo "Users:     ${USERS}"
echo ""

# Step 1: Set worker count
echo "==> Setting worker count to ${WORKERS}..."
aws ecs update-service \
  --cluster "${ECS_CLUSTER}" \
  --service "${WORKER_SERVICE}" \
  --desired-count "${WORKERS}" \
  --region "${AWS_REGION}" \
  --no-cli-pager > /dev/null

echo "==> Waiting for ECS to stabilize..."
aws ecs wait services-stable \
  --cluster "${ECS_CLUSTER}" \
  --services "${WORKER_SERVICE}" \
  --region "${AWS_REGION}" 2>/dev/null || sleep 90

# Verify workers are running
RUNNING=$(aws ecs describe-services \
  --cluster "${ECS_CLUSTER}" \
  --services "${WORKER_SERVICE}" \
  --region "${AWS_REGION}" \
  --query 'services[0].runningCount' \
  --output text)
echo "==> Workers running: ${RUNNING}"

# Step 2: Flush Redis
if [[ -n "$REDIS_HOST" ]]; then
  echo "==> Flushing Redis..."
  redis-cli -h "${REDIS_HOST}" -p "${REDIS_PORT}" FLUSHDB || echo "    (redis-cli not available, flush manually or skip)"
fi

# Step 3: Health check
echo "==> Health check..."
curl -fsS "${API_URL}/health"
echo ""

# Step 4: Clear previous files
mkdir -p "${RESULTS_DIR}"
> "${JOB_ID_FILE}"

# Step 5: Start metrics polling in background BEFORE the burst
echo ""
echo "==> Starting metrics polling..."
python3 "${PROJECT_ROOT}/analysis/run_scaling_experiment.py" \
  --api-base-url "${API_URL}" \
  --job-id-file "${JOB_ID_FILE}" \
  --worker-count "${WORKERS}" \
  --label "${LABEL}" \
  --mode poll-metrics &
METRICS_PID=$!

# Give the metrics poller a moment to start
sleep 1

# Step 6: Run Locust burst
echo "==> Running Locust burst: ${USERS} users..."
VIDEO_DIR="${VIDEO_DIR}" JOB_ID_FILE="${JOB_ID_FILE}" \
  locust -f "${PROJECT_ROOT}/analysis/locustfile.py" \
    --host "${API_URL}" \
    --users "${USERS}" \
    --spawn-rate "${USERS}" \
    --run-time 300s \
    --headless \
    --only-summary 2>&1 | tee "${RESULTS_DIR}/locust_output.log" | tail -30

JOB_COUNT=$(wc -l < "${JOB_ID_FILE}" | tr -d ' ')
echo ""
echo "==> ${JOB_COUNT} jobs submitted."

if [[ "$JOB_COUNT" -lt "$USERS" ]]; then
  echo "WARNING: only ${JOB_COUNT}/${USERS} jobs submitted. Some uploads may have timed out."
fi

# Step 7: Stop metrics polling, then collect final results
kill "${METRICS_PID}" 2>/dev/null || true
wait "${METRICS_PID}" 2>/dev/null || true

echo ""
echo "==> Collecting results..."
python3 "${PROJECT_ROOT}/analysis/run_scaling_experiment.py" \
  --api-base-url "${API_URL}" \
  --job-id-file "${JOB_ID_FILE}" \
  --worker-count "${WORKERS}" \
  --label "${LABEL}" \
  --mode collect

echo ""
echo "Phase A run complete for ${WORKERS} workers."
