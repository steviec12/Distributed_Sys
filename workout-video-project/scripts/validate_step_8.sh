#!/usr/bin/env bash
set -euo pipefail

PROJECT_ROOT="/Users/stevi/Documents/web-service-gin/workout-video-project"
TERRAFORM_DIR="$PROJECT_ROOT/infra/terraform"
TF_DATA_DIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TF_DATA_DIR"
}

trap cleanup EXIT

echo "Step 8 core validation starting."

if ! command -v terraform >/dev/null 2>&1; then
  echo "terraform is required for Step 8 validation."
  exit 1
fi

echo ""
echo "==> Checking Terraform formatting"
terraform -chdir="$TERRAFORM_DIR" fmt -check -recursive

echo ""
echo "==> Initializing Terraform providers (backend disabled)"
TF_DATA_DIR="$TF_DATA_DIR" terraform -chdir="$TERRAFORM_DIR" init -backend=false -input=false

echo ""
echo "==> Validating Terraform configuration"
TF_DATA_DIR="$TF_DATA_DIR" terraform -chdir="$TERRAFORM_DIR" validate

echo ""
echo "Step 8 Terraform validation passed."

echo ""
echo "Step 8 core validation finished."
