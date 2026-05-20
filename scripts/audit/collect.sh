#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

ensure_lune_container

stamp="$(timestamp_utc)"
host_out="$AUDIT_OUTPUT_DIR/$stamp"
container_out="/out/$stamp"
suffix=0
while [[ -e "$host_out" ]]; do
  suffix=$((suffix + 1))
  host_out="$AUDIT_OUTPUT_DIR/${stamp}-$suffix"
  container_out="/out/${stamp}-$suffix"
done
mkdir -p "$host_out"

"$AUDIT_SCRIPT_DIR/run-tools.sh" /work/scripts/audit/tool-collect.sh "$container_out"

echo "Audit evidence written to $host_out"
