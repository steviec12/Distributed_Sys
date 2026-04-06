#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HW10_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
TARGETS_FILE="${HW10_DIR}/cloud_targets.env"

if [[ ! -f "${TARGETS_FILE}" ]]; then
  echo "Missing ${TARGETS_FILE}" >&2
  exit 1
fi

# shellcheck disable=SC1090
source "${TARGETS_FILE}"

MODE="${1:-}"
W_VALUE="${2:-}"
R_VALUE="${3:-}"

if [[ -z "${MODE}" ]]; then
  echo "Usage:" >&2
  echo "  ${0} leader_follower <W> <R>" >&2
  echo "  ${0} leaderless" >&2
  exit 1
fi

case "${MODE}" in
  leader_follower)
    if [[ -z "${W_VALUE}" || -z "${R_VALUE}" ]]; then
      echo "leader_follower mode requires W and R values" >&2
      exit 1
    fi
    ;;
  leaderless)
    W_VALUE="5"
    R_VALUE="1"
    ;;
  *)
    echo "Unsupported mode: ${MODE}" >&2
    exit 1
    ;;
esac

if [[ ! -f "${PEM_PATH}" ]]; then
  echo "PEM file not found: ${PEM_PATH}" >&2
  exit 1
fi

REMOTE_DIR="/home/ec2-user/hw10-app"
IMAGE_NAME="hw10-node"
STAGING_DIR="$(mktemp -d /tmp/hw10-deploy.XXXXXX)"
IMAGE_TAR="${STAGING_DIR}/hw10-node.tar"
SSH_OPTS=(
  -o StrictHostKeyChecking=no
  -o IdentitiesOnly=yes
  -o BatchMode=yes
  -o ConnectTimeout=15
  -i "${PEM_PATH}"
)

cleanup() {
  rm -rf "${STAGING_DIR}"
}

trap cleanup EXIT

NODE_NAMES=(node1 node2 node3 node4 node5)
NODE_IDS=(node-1 node-2 node-3 node-4 node-5)
PUBLIC_IPS=(
  "${DB_NODE1_PUBLIC_IP}"
  "${DB_NODE2_PUBLIC_IP}"
  "${DB_NODE3_PUBLIC_IP}"
  "${DB_NODE4_PUBLIC_IP}"
  "${DB_NODE5_PUBLIC_IP}"
)
PRIVATE_URLS=(
  "${DB_NODE1_PRIVATE_URL}"
  "${DB_NODE2_PRIVATE_URL}"
  "${DB_NODE3_PRIVATE_URL}"
  "${DB_NODE4_PRIVATE_URL}"
  "${DB_NODE5_PRIVATE_URL}"
)

FOLLOWER_ADDRS="${DB_NODE2_PRIVATE_URL},${DB_NODE3_PRIVATE_URL},${DB_NODE4_PRIVATE_URL},${DB_NODE5_PRIVATE_URL}"

peer_addrs_for_index() {
  local target_index="$1"
  local values=()
  local idx
  for idx in "${!PRIVATE_URLS[@]}"; do
    if [[ "${idx}" != "${target_index}" ]]; then
      values+=("${PRIVATE_URLS[$idx]}")
    fi
  done
  local joined=""
  local value
  for value in "${values[@]}"; do
    if [[ -n "${joined}" ]]; then
      joined+=","
    fi
    joined+="${value}"
  done
  printf '%s' "${joined}"
}

run_ssh() {
  local host="$1"
  shift
  ssh "${SSH_OPTS[@]}" "ec2-user@${host}" "$@"
}

prepare_image() {
  echo "Building Docker image locally"
  docker build --platform linux/amd64 -t "${IMAGE_NAME}" "${HW10_DIR}"
  echo "Saving Docker image locally"
  docker save -o "${IMAGE_TAR}" "${IMAGE_NAME}"
}

copy_image_to_node() {
  local host="$1"
  run_ssh "${host}" "mkdir -p ${REMOTE_DIR}"
  scp "${SSH_OPTS[@]}" "${IMAGE_TAR}" "ec2-user@${host}:${REMOTE_DIR}/hw10-node.tar"
}

deploy_node() {
  local index="$1"
  local name="${NODE_NAMES[$index]}"
  local node_id="${NODE_IDS[$index]}"
  local host="${PUBLIC_IPS[$index]}"
  local env_file
  env_file="$(mktemp)"

  {
    echo "PORT=8080"
    echo "NODE_ID=${node_id}"
    echo "N=5"
    echo "W=${W_VALUE}"
    echo "R=${R_VALUE}"
    if [[ "${MODE}" == "leader_follower" ]]; then
      if [[ "${index}" == "0" ]]; then
        echo "ROLE=leader"
        echo "MODE=leader_follower"
        echo "LEADER_ADDR=${DB_NODE1_PRIVATE_URL}"
        echo "FOLLOWER_ADDRS=${FOLLOWER_ADDRS}"
      else
        echo "ROLE=follower"
        echo "MODE=leader_follower"
        echo "LEADER_ADDR=${DB_NODE1_PRIVATE_URL}"
      fi
    else
      echo "ROLE=replica"
      echo "MODE=leaderless"
      echo "PEER_ADDRS=$(peer_addrs_for_index "${index}")"
    fi
  } >"${env_file}"

  echo "Deploying ${name} (${host})"
  copy_image_to_node "${host}"
  scp "${SSH_OPTS[@]}" "${env_file}" "ec2-user@${host}:${REMOTE_DIR}/node.env"

  run_ssh "${host}" "
    sudo docker rm -f hw10-node >/dev/null 2>&1 || true
    sudo docker image rm -f ${IMAGE_NAME} >/dev/null 2>&1 || true
    sudo docker load -i ${REMOTE_DIR}/hw10-node.tar
    rm -f ${REMOTE_DIR}/hw10-node.tar
    sudo docker run -d --name hw10-node --restart unless-stopped --env-file ${REMOTE_DIR}/node.env -p 8080:8080 hw10-node
  "

  rm -f "${env_file}"
}

prepare_image

for idx in "${!NODE_NAMES[@]}"; do
  deploy_node "${idx}"
done

echo "Deployment complete for mode=${MODE} w=${W_VALUE} r=${R_VALUE}"
