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

debug_host_dir="$host_out/redacted/lune-debug"
mkdir -p "$debug_host_dir"
for cmd in summary recent-operations db-integrity cpa-auth cpa-runtime; do
  if docker exec "$LUNE_CONTAINER" lune debug "$cmd" > "$debug_host_dir/$cmd.json" 2>"$debug_host_dir/$cmd.err"; then
    rm -f "$debug_host_dir/$cmd.err"
  fi
done
operation_id="$(docker exec "$LUNE_CONTAINER" sh -c 'LUNE_DATA_DIR="${LUNE_DATA_DIR:-/app/data}" lune debug recent-operations' 2>/dev/null \
  | sed -n 's/.*"operation_id": "\([^"]*\)".*/\1/p' | head -n 1 || true)"
if [[ -n "$operation_id" ]]; then
  docker exec "$LUNE_CONTAINER" lune debug operation "$operation_id" > "$debug_host_dir/operation.json" 2>"$debug_host_dir/operation.err" \
    || true
  [[ -s "$debug_host_dir/operation.err" ]] || rm -f "$debug_host_dir/operation.err"
fi
account_id="$(docker exec "$LUNE_CONTAINER" sh -c 'LUNE_DATA_DIR="${LUNE_DATA_DIR:-/app/data}" lune debug summary' 2>/dev/null \
  | sed -n 's/.*"first_account_id": \([0-9][0-9]*\).*/\1/p' | head -n 1 || true)"
if [[ -n "$account_id" ]]; then
  docker exec "$LUNE_CONTAINER" lune debug account "$account_id" > "$debug_host_dir/account-$account_id.json" 2>"$debug_host_dir/account-$account_id.err" \
    || true
  [[ -s "$debug_host_dir/account-$account_id.err" ]] || rm -f "$debug_host_dir/account-$account_id.err"
fi

echo "Audit evidence written to $host_out"
