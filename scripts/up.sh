#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

read_config_token() {
  if [[ -f configs/config.json ]]; then
    sed -n 's/.*"admin_token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' configs/config.json | head -n1
  fi
}

append_env() {
  local key="$1"
  local value="$2"
  touch .env
  if grep -q "^${key}=" .env; then
    return 0
  fi
  printf '\n%s=%s\n' "$key" "$value" >> .env
}

generate_token() {
  if command -v openssl >/dev/null 2>&1; then
    printf 'lune-%s' "$(openssl rand -hex 24)"
  else
    printf 'lune-%s' "$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  fi
}

wait_for_http() {
  local url="$1"
  local name="$2"
  local attempts="${3:-60}"
  local i
  for ((i = 1; i <= attempts; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf '%s did not become ready: %s\n' "$name" "$url" >&2
  return 1
}

LUNE_ADMIN_TOKEN="${LUNE_ADMIN_TOKEN:-}"
if [[ -z "$(trim "$LUNE_ADMIN_TOKEN")" ]]; then
  LUNE_ADMIN_TOKEN="$(trim "$(read_config_token)")"
fi
if [[ -z "$(trim "$LUNE_ADMIN_TOKEN")" ]]; then
  LUNE_ADMIN_TOKEN="$(generate_token)"
  append_env "LUNE_ADMIN_TOKEN" "$LUNE_ADMIN_TOKEN"
fi
export LUNE_ADMIN_TOKEN

LUNE_BACKEND_ADMIN_USERNAME="${LUNE_BACKEND_ADMIN_USERNAME:-root}"
export LUNE_BACKEND_ADMIN_USERNAME
append_env "LUNE_BACKEND_ADMIN_USERNAME" "$LUNE_BACKEND_ADMIN_USERNAME"

if [[ -z "${LUNE_BACKEND_KEY:-}" ]]; then
  printf 'Missing LUNE_BACKEND_KEY. Set it in .env before running %s\n' "$0" >&2
  exit 1
fi

if [[ -z "${LUNE_BACKEND_ADMIN_PASSWORD:-}" ]]; then
  printf 'Missing LUNE_BACKEND_ADMIN_PASSWORD. Set the backend admin password in .env before running %s\n' "$0" >&2
  exit 1
fi

docker compose up -d backend lune

wait_for_http "http://localhost:3000/api/status" "backend"
wait_for_http "http://localhost:7788/healthz" "lune"

cat <<EOF
Lune is ready.

Frontend:
  http://localhost:7788/admin

Backend:
  http://localhost:3000

Login with this Lune Admin Token:
  $LUNE_ADMIN_TOKEN

Backend admin session is handled by Lune automatically.
EOF
