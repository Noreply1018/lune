#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ensure_lune_container

mkdir -p "$AUDIT_OUTPUT_DIR"
data_volume="$(resolve_lune_data_volume)"

if [[ "$#" -eq 0 ]]; then
  set -- bash
fi

tty_args=()
if [[ -t 0 && -t 1 ]]; then
  tty_args=(-it)
fi

docker run --rm "${tty_args[@]}" \
  --name "lune-audit-tools-$(timestamp_utc)-$$" \
  --network "container:$LUNE_CONTAINER" \
  --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,noexec,size=64m \
  --user "$(id -u):$(id -g)" \
  -e "LUNE_CONTAINER=$LUNE_CONTAINER" \
  -e "LUNE_DATA_DIR=/data" \
  -e "LUNE_CPA_FILES_DIR=/data/cpa-auth" \
  -e "LUNE_CPA_BASE_URL=http://127.0.0.1:8317" \
  -e "LUNE_AUDIT_OUT=/out" \
  -v "$data_volume:/data:ro" \
  -v "$REPO_ROOT:/work:ro" \
  -v "$AUDIT_OUTPUT_DIR:/out" \
  "$AUDIT_TOOLS_IMAGE" \
  "$@"
