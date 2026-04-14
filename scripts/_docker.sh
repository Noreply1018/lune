#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

# Export LUNE_PORT for docker-compose.yml variable substitution.
# Priority: env var > lune.yaml > default 7788
if [[ -z "${LUNE_PORT:-}" ]] && [[ -f lune.yaml ]]; then
  _port="$(grep -E '^[[:space:]]*port[[:space:]]*:' lune.yaml | head -1 | sed 's/.*:[[:space:]]*//' | tr -d '[:space:]')"
  if [[ -n "$_port" ]]; then
    export LUNE_PORT="$_port"
  fi
fi
export LUNE_PORT="${LUNE_PORT:-7788}"

resolve_lune_port() {
  local port
  port="$(docker compose port lune 7788 2>/dev/null | awk -F: 'NF{print $NF}' | tail -n 1)"
  if [[ -n "$port" ]]; then
    echo "$port"
    return
  fi

  if [[ -n "${LUNE_PORT:-}" ]]; then
    echo "$LUNE_PORT"
    return
  fi

  if [[ -f lune.yaml ]]; then
    port="$(grep -E '^[[:space:]]*port[[:space:]]*:' lune.yaml | head -1 | sed 's/.*:[[:space:]]*//' | tr -d '[:space:]')"
    if [[ -n "$port" ]]; then
      echo "$port"
      return
    fi
  fi

  echo "7788"
}

print_access_hint() {
  local port
  port="$(resolve_lune_port)"

  echo
  echo "Lune is running"
  echo
  echo "  Admin UI:    http://127.0.0.1:${port}/admin"
  echo "  Gateway API: http://127.0.0.1:${port}/v1"
  echo
  echo "Useful commands:"
  echo "  ./scripts/ps.sh"
  echo "  ./scripts/logs.sh"
  echo "  ./scripts/restart.sh"
  echo
}
