#!/usr/bin/env bash
set -euo pipefail

AUDIT_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$AUDIT_SCRIPT_DIR/../.." && pwd)"

AUDIT_TOOLS_IMAGE="${LUNE_AUDIT_TOOLS_IMAGE:-lune-audit-tools:local}"
LUNE_CONTAINER="${LUNE_CONTAINER:-lune}"
LUNE_DATA_VOLUME="${LUNE_DATA_VOLUME:-}"
AUDIT_OUTPUT_DIR="${LUNE_AUDIT_OUTPUT_DIR:-$REPO_ROOT/audit-output}"

die() {
  echo "error: $*" >&2
  exit 1
}

ensure_docker() {
  command -v docker >/dev/null 2>&1 || die "docker is required"
}

ensure_lune_container() {
  ensure_docker
  docker container inspect "$LUNE_CONTAINER" >/dev/null 2>&1 \
    || die "Lune container '$LUNE_CONTAINER' was not found; set LUNE_CONTAINER to override"
}

resolve_lune_data_volume() {
  if [[ -n "$LUNE_DATA_VOLUME" ]]; then
    echo "$LUNE_DATA_VOLUME"
    return
  fi

  local volume
  volume="$(docker inspect "$LUNE_CONTAINER" \
    --format '{{range .Mounts}}{{if and (eq .Type "volume") (eq .Destination "/app/data")}}{{.Name}}{{end}}{{end}}' \
    2>/dev/null)"
  if [[ -z "$volume" ]]; then
    die "could not resolve data volume for '$LUNE_CONTAINER'; set LUNE_DATA_VOLUME explicitly"
  fi
  echo "$volume"
}

timestamp_utc() {
  date -u +"%Y%m%dT%H%M%SZ"
}
