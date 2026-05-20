#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ensure_docker

docker build \
  -t "$AUDIT_TOOLS_IMAGE" \
  -f "$REPO_ROOT/docker/audit-tools/Dockerfile" \
  "$REPO_ROOT"

echo "Built $AUDIT_TOOLS_IMAGE"
