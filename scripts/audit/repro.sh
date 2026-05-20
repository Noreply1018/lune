#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

scenario="${1:-}"
[[ -n "$scenario" ]] || die "Usage: scripts/audit/repro.sh <scenario>"

validate_repro_output() {
  local scenario_name="$1"
  local host_out="$2"
  local scenario_dir="$host_out/redacted/scenario"
  local summary="$scenario_dir/$scenario_name-summary.json"
  [[ -s "$summary" ]] || die "missing scenario summary: $summary"

  case "$scenario_name" in
    cpa-import-reimport)
      grep -q '"status": "ok"' "$summary" || die "scenario summary did not report ok: $summary"
      for file in \
        cpa-import-reimport-import-response.json \
        cpa-import-reimport-delete-response.json \
        cpa-import-reimport-reimport-response.json \
        cpa-import-reimport-operations.json \
        cpa-import-reimport-items.json \
        cpa-import-reimport-account.json; do
        [[ -s "$scenario_dir/$file" ]] || die "missing cpa-import-reimport evidence: $file"
      done
      grep -q '"initial_batch_id":' "$summary" || die "missing initial batch id in $summary"
      grep -q '"reimport_batch_id":' "$summary" || die "missing reimport batch id in $summary"
      ;;
    stateful-probe)
      grep -Eq '"status": "(evidence_collected|request_log_collected)"' "$summary" || die "stateful-probe summary did not report collected evidence state: $summary"
      [[ -s "$scenario_dir/stateful-probe-log.json" ]] || die "missing stateful-probe log evidence"
      [[ -s "$scenario_dir/stateful-probe-account.json" ]] || die "missing stateful-probe account evidence"
      [[ -s "$scenario_dir/stateful-probe-diagnostic.json" ]] || die "missing stateful-probe diagnostic evidence"
      [[ -f "$scenario_dir/stateful-probe-diagnostic-evidence.json" ]] || die "missing stateful-probe diagnostic item evidence"
      grep -q '"traffic_kind":' "$scenario_dir/stateful-probe-log.json" || die "stateful-probe log missing traffic kind"
      ;;
  esac
}

run_repro_on_container() {
  local scenario_name="$1"
  local target_container="$2"
  local target_volume="$3"
  local import_dir="${LUNE_AUDIT_CPA_IMPORT_DIR:-}"
  local stamp host_out container_out suffix
  stamp="$(timestamp_utc)"
  host_out="$AUDIT_OUTPUT_DIR/repro-$scenario_name-$stamp"
  container_out="/out/repro-$scenario_name-$stamp"
  suffix=0
  while [[ -e "$host_out" ]]; do
    suffix=$((suffix + 1))
    host_out="$AUDIT_OUTPUT_DIR/repro-$scenario_name-$stamp-$suffix"
    container_out="/out/repro-$scenario_name-$stamp-$suffix"
  done
  mkdir -p "$host_out"

  LUNE_CONTAINER="$target_container" LUNE_DATA_VOLUME="$target_volume" LUNE_AUDIT_CPA_IMPORT_DIR="$import_dir" \
    "$AUDIT_SCRIPT_DIR/run-tools.sh" bash /work/scripts/audit/tool-repro.sh "$scenario_name" "$container_out"
  LUNE_CONTAINER="$target_container" LUNE_DATA_VOLUME="$target_volume" LUNE_AUDIT_CPA_IMPORT_DIR="$import_dir" \
    "$AUDIT_SCRIPT_DIR/run-tools.sh" /work/scripts/audit/tool-collect.sh "$container_out"
  validate_repro_output "$scenario_name" "$host_out"
  echo "Repro evidence written to $host_out"
}

wait_for_lune_container() {
  local container="$1"
  for _ in $(seq 1 60); do
    if docker exec "$container" sh -c 'curl -fsS "http://127.0.0.1:${LUNE_PORT:-7788}/healthz" >/dev/null' >/dev/null 2>&1; then
      return
    fi
    sleep 1
  done
  docker logs --tail 80 "$container" >&2 || true
  die "temporary Lune container did not become ready: $container"
}

run_isolated_repro() {
  local scenario_name="$1"
  ensure_lune_container

  local source_container source_volume source_image stamp temp_volume temp_container env_file
  source_container="$LUNE_CONTAINER"
  source_volume="$(resolve_lune_data_volume)"
  source_image="${LUNE_REPRO_IMAGE:-$(docker inspect "$source_container" --format '{{.Config.Image}}')}"
  stamp="$(timestamp_utc)"
  temp_volume="lune-audit-repro-${scenario_name}-${stamp}-$$"
  temp_container="lune-audit-repro-${scenario_name}-${stamp}-$$"
  env_file="$(mktemp)"

  cleanup_isolated_best_effort() {
    [[ -n "${temp_container:-}" ]] && docker rm -f "$temp_container" >/dev/null 2>&1 || true
    [[ -n "${temp_volume:-}" ]] && docker volume rm "$temp_volume" >/dev/null 2>&1 || true
    [[ -n "${env_file:-}" ]] && rm -f "$env_file"
  }
  cleanup_isolated_strict() {
    local failed=0
    if [[ -n "${temp_container:-}" ]]; then
      docker rm -f "$temp_container" >/dev/null || failed=1
      temp_container=""
    fi
    if [[ -n "${temp_volume:-}" ]]; then
      docker volume rm "$temp_volume" >/dev/null || failed=1
      temp_volume=""
    fi
    if [[ -n "${env_file:-}" ]]; then
      rm -f "$env_file" || failed=1
      env_file=""
    fi
    return "$failed"
  }
  trap cleanup_isolated_best_effort EXIT

  docker volume create "$temp_volume" >/dev/null
  docker run --rm \
    -v "$source_volume:/from:ro" \
    -v "$temp_volume:/to" \
    alpine:3.20 \
    sh -c 'cd /from && tar cf - . | tar xpf - -C /to'

  docker inspect "$source_container" --format '{{range .Config.Env}}{{println .}}{{end}}' > "$env_file"
  docker run -d \
    --name "$temp_container" \
    --env-file "$env_file" \
    -v "$temp_volume:/app/data" \
    "$source_image" >/dev/null

  wait_for_lune_container "$temp_container"
  if [[ "$scenario_name" == "cpa-import-reimport" ]]; then
    docker exec "$temp_container" sh -c 'set -eu
      rm -rf /app/data/audit-cpa-import
      mkdir -p /app/data/audit-cpa-import
      count=0
      for file in /app/data/cpa-auth/*.json; do
        [ -f "$file" ] || continue
        [ "$(basename "$file")" = ".login-sessions.json" ] && continue
        cp "$file" /app/data/audit-cpa-import/
        count=$((count + 1))
      done
      [ "$count" -gt 0 ]
      chmod 755 /app/data/audit-cpa-import
      chmod 644 /app/data/audit-cpa-import/*.json'
    LUNE_AUDIT_CPA_IMPORT_DIR=/data/audit-cpa-import run_repro_on_container "$scenario_name" "$temp_container" "$temp_volume"
  else
    run_repro_on_container "$scenario_name" "$temp_container" "$temp_volume"
  fi

  cleanup_isolated_strict || die "failed to clean temporary repro container or volume"
  trap - EXIT
}

case "$scenario" in
  cpa-import-reimport)
    run_isolated_repro "$scenario"
    ;;
  stateful-probe)
    run_isolated_repro "$scenario"
    ;;
  runtime-reload|pool-delete|quota-refresh|routing-failover)
    die "scenario_not_implemented: $scenario requires isolated fixture data and API choreography before it can be run safely"
    ;;
  *)
    die "unknown scenario: $scenario"
    ;;
esac
