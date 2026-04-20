#!/usr/bin/env bash
set -euo pipefail

# Phase C: Crash Recovery Under Load
#
# Usage:
#   bash analysis/run_crash_experiment_aws.sh \
#     --api-url http://<ALB_URL> \
#     --workers 4 \
#     --kill-after 60 \
#     --ec2-host ec2-user@35.88.33.55 \
#     --ec2-key ~/Documents/my-ec2-key.pem \
#     --redis-host workout-video-redis.odjz60.0001.usw2.cache.amazonaws.com

API_URL=""
WORKERS=""
KILL_AFTER="60"
EC2_HOST=""
EC2_KEY=""
REDIS_HOST=""
REDIS_PORT="6379"
ECS_CLUSTER="workout-video-cluster"
WORKER_SERVICE="workout-video-worker"
AWS_REGION="us-west-2"
LOCUST_USERS="240"
LOCUST_SPAWN_RATE="2"
LOCUST_RUN_TIME="120s"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api-url) API_URL="$2"; shift 2 ;;
    --workers) WORKERS="$2"; shift 2 ;;
    --kill-after) KILL_AFTER="$2"; shift 2 ;;
    --ec2-host) EC2_HOST="$2"; shift 2 ;;
    --ec2-key) EC2_KEY="$2"; shift 2 ;;
    --redis-host) REDIS_HOST="$2"; shift 2 ;;
    --redis-port) REDIS_PORT="$2"; shift 2 ;;
    --cluster) ECS_CLUSTER="$2"; shift 2 ;;
    --worker-service) WORKER_SERVICE="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if [[ -z "$API_URL" || -z "$WORKERS" || -z "$EC2_HOST" || -z "$EC2_KEY" ]]; then
  echo "Required: --api-url, --workers, --ec2-host, --ec2-key"
  exit 1
fi

METRICS_PID=""
LOCUST_SSH_PID=""
cleanup() {
  [[ -n "$METRICS_PID" ]] && kill "$METRICS_PID" 2>/dev/null || true
  [[ -n "$LOCUST_SSH_PID" ]] && kill "$LOCUST_SSH_PID" 2>/dev/null || true
}
trap cleanup EXIT

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
RESULTS_DIR="${PROJECT_ROOT}/analysis/results"
JOB_ID_FILE="${RESULTS_DIR}/job_ids.txt"
LABEL="${WORKERS}w_crash"

echo "============================================================"
echo "Phase C: Crash Recovery — ${WORKERS} workers, kill after ${KILL_AFTER}s"
echo "============================================================"
echo "API:        ${API_URL}"
echo "EC2:        ${EC2_HOST}"
echo "Locust:     ${LOCUST_USERS} users, spawn-rate ${LOCUST_SPAWN_RATE}, run-time ${LOCUST_RUN_TIME}"
echo "Kill after: ${KILL_AFTER}s"
echo ""

# Step 1: Set worker count
echo "[Step 1/12] Setting worker count to ${WORKERS}..."
aws ecs update-service \
  --cluster "${ECS_CLUSTER}" \
  --service "${WORKER_SERVICE}" \
  --desired-count "${WORKERS}" \
  --region "${AWS_REGION}" \
  --no-cli-pager > /dev/null

echo "[Step 1/12] Waiting for ECS to stabilize..."
aws ecs wait services-stable \
  --cluster "${ECS_CLUSTER}" \
  --services "${WORKER_SERVICE}" \
  --region "${AWS_REGION}" 2>/dev/null || sleep 90

RUNNING=$(aws ecs describe-services \
  --cluster "${ECS_CLUSTER}" \
  --services "${WORKER_SERVICE}" \
  --region "${AWS_REGION}" \
  --query 'services[0].runningCount' \
  --output text)
echo "[Step 1/12] Workers running: ${RUNNING}"

# Step 2: Flush Redis
echo "[Step 2/12] Flushing Redis via EC2..."
if [[ -n "$REDIS_HOST" ]]; then
  ssh -i "${EC2_KEY}" -o StrictHostKeyChecking=no "${EC2_HOST}" \
    "redis6-cli -h ${REDIS_HOST} -p ${REDIS_PORT} FLUSHDB" || echo "    (flush failed, continuing)"
else
  echo "    WARNING: --redis-host not set, skipping flush"
fi

# Step 3: Health check
echo "[Step 3/12] Health check..."
curl -fsS "${API_URL}/health"
echo ""

# Step 4: Clear previous files
echo "[Step 4/12] Clearing previous job IDs..."
mkdir -p "${RESULTS_DIR}"
> "${JOB_ID_FILE}"

# Step 5: Start metrics poller in background
echo "[Step 5/12] Starting metrics poller..."
python3 "${PROJECT_ROOT}/analysis/run_sustained_load_experiment.py" \
  --api-base-url "${API_URL}" \
  --job-id-file "${JOB_ID_FILE}" \
  --worker-count "${WORKERS}" \
  --rate 100 \
  --label "${LABEL}" \
  --mode poll-metrics &
METRICS_PID=$!
sleep 1

# Step 6: Start Locust on EC2 in background
echo "[Step 6/12] Starting Locust on EC2 (${LOCUST_USERS} users, ${LOCUST_RUN_TIME})..."
ssh -i "${EC2_KEY}" -o StrictHostKeyChecking=no -o ServerAliveInterval=30 -o ServerAliveCountMax=3 "${EC2_HOST}" \
  "cd ~ && > ~/job_ids.txt && VIDEO_DIR=~/videos JOB_ID_FILE=~/job_ids.txt locust -f ~/locustfile.py --host ${API_URL} --users ${LOCUST_USERS} --spawn-rate ${LOCUST_SPAWN_RATE} --run-time ${LOCUST_RUN_TIME} --headless > ~/locust_output.log 2>&1" &
LOCUST_SSH_PID=$!

# Step 7: Countdown to kill
echo "[Step 7/12] Waiting ${KILL_AFTER}s before killing a worker..."
for ((i = KILL_AFTER; i > 0; i -= 10)); do
  sleep 10
  REMAINING=$((i - 10))
  if (( REMAINING > 0 )); then
    CURRENT_METRICS=$(curl -fsS "${API_URL}/metrics" 2>/dev/null || echo "unavailable")
    echo "    ${REMAINING}s until kill... metrics: ${CURRENT_METRICS}"
  fi
done

# Step 8: Record pre-kill metrics BEFORE the kill
PRE_KILL_METRICS=$(curl -fsS "${API_URL}/metrics" 2>/dev/null || echo "{}")
echo "[Step 8/12] Pre-kill metrics: ${PRE_KILL_METRICS}"

TASK_ID=$(aws ecs list-tasks \
  --cluster "${ECS_CLUSTER}" \
  --service-name "${WORKER_SERVICE}" \
  --region "${AWS_REGION}" \
  --query 'taskArns[0]' \
  --output text)

KILL_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
echo "[Step 8/12] KILLING worker task: ${TASK_ID}"
echo "[Step 8/12] Kill time: ${KILL_TIME}"
aws ecs stop-task \
  --cluster "${ECS_CLUSTER}" \
  --task "${TASK_ID}" \
  --reason "Phase C crash experiment" \
  --region "${AWS_REGION}" \
  --no-cli-pager > /dev/null
echo "[Step 8/12] Worker killed. ECS will restart it automatically."

# Step 9: Monitor while Locust runs
echo "[Step 9/12] Monitoring while Locust continues..."
MONITOR_COUNT=0
while kill -0 "${LOCUST_SSH_PID}" 2>/dev/null; do
  sleep 15
  MONITOR_COUNT=$((MONITOR_COUNT + 1))
  LIVE_METRICS=$(curl -fsS "${API_URL}/metrics" 2>/dev/null || echo "unavailable")
  echo "    [monitor ${MONITOR_COUNT}] ${LIVE_METRICS}"
done
echo "[Step 9/12] Locust finished."
wait "${LOCUST_SSH_PID}" 2>/dev/null || echo "[Step 9/12] WARNING: Locust SSH exited non-zero"

# Step 10: Stop metrics poller
echo "[Step 10/12] Stopping metrics poller..."
kill "${METRICS_PID}" 2>/dev/null || true
wait "${METRICS_PID}" 2>/dev/null || true

# Step 11: Copy job IDs from EC2
echo "[Step 11/12] Copying job IDs from EC2..."
scp -i "${EC2_KEY}" -o StrictHostKeyChecking=no "${EC2_HOST}:~/job_ids.txt" "${JOB_ID_FILE}"

JOB_COUNT=$(wc -l < "${JOB_ID_FILE}" | tr -d ' ')
echo "[Step 11/12] ${JOB_COUNT} jobs submitted."

# Step 12: Collect results
echo ""
echo "[Step 12/12] Collecting results..."
python3 "${PROJECT_ROOT}/analysis/run_sustained_load_experiment.py" \
  --api-base-url "${API_URL}" \
  --job-id-file "${JOB_ID_FILE}" \
  --worker-count "${WORKERS}" \
  --rate 100 \
  --label "${LABEL}" \
  --mode collect \
  --completion-timeout 1800

# Save crash metadata
RESULT_DIRS=$(find "${PROJECT_ROOT}/analysis/results/sustained" -name "${LABEL}" -type d | sort | tail -1)
if [[ -n "${RESULT_DIRS}" ]]; then
  POST_KILL_METRICS=$(curl -fsS "${API_URL}/metrics" 2>/dev/null || echo "{}")
  cat > "${RESULT_DIRS}/crash_metadata.json" <<METAEOF
{
  "experiment": "phase_c_crash_recovery",
  "worker_count": ${WORKERS},
  "kill_after_seconds": ${KILL_AFTER},
  "kill_time": "${KILL_TIME}",
  "killed_task_id": "${TASK_ID}",
  "pre_kill_metrics": ${PRE_KILL_METRICS},
  "post_experiment_metrics": ${POST_KILL_METRICS}
}
METAEOF
  echo "==> Crash metadata saved to ${RESULT_DIRS}/crash_metadata.json"
else
  echo "WARNING: no result directory found, crash metadata not saved"
fi

echo ""
echo "Phase C run complete for ${WORKERS} workers."
