#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$REPO_ROOT"

# Docker Compose 会自动读取 REPO_ROOT/.env，无需手动 export。
# 这里只有 resolve_lune_port 在容器还没起来时需要预测端口，它会按顺序查：
#   docker compose port lune 7788 → shell $LUNE_PORT → .env 文件里的 LUNE_PORT → 7788

_env_lookup() {
  # 从 REPO_ROOT/.env 读取指定 key 的值（忽略注释、空行、引号）
  local key="$1"
  [[ -f .env ]] || return 0
  grep -E "^[[:space:]]*${key}[[:space:]]*=" .env \
    | tail -n 1 \
    | sed -E "s/^[[:space:]]*${key}[[:space:]]*=[[:space:]]*//; s/[[:space:]]+#.*\$//; s/^['\"]//; s/['\"][[:space:]]*\$//; s/[[:space:]]*\$//"
}

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

  port="$(_env_lookup LUNE_PORT)"
  if [[ -n "$port" ]]; then
    echo "$port"
    return
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
