#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT_DIR="${1:-$ROOT_DIR/dist/lambda}"

mkdir -p "$OUT_DIR"

cd "$ROOT_DIR"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -tags lambda.norpc -ldflags="-s -w" -o "$OUT_DIR/bootstrap" ./cmd/lambda

(
  cd "$OUT_DIR"
  zip -q lambda.zip bootstrap
)

echo "$OUT_DIR/lambda.zip"
