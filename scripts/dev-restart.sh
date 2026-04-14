#!/usr/bin/env bash
set -euo pipefail

# 从 LUNE_PORT / lune.yaml / 默认值 解析当前端口
resolve_port() {
  if [[ -n "${LUNE_PORT:-}" ]]; then
    echo "$LUNE_PORT"
    return
  fi
  if command -v yq &>/dev/null && [[ -f lune.yaml ]]; then
    local p
    p=$(yq '.port // 7788' lune.yaml 2>/dev/null)
    echo "$p"
    return
  fi
  if [[ -f lune.yaml ]]; then
    local p
    p=$(grep -E '^\s*port\s*:' lune.yaml | head -1 | sed 's/.*:\s*//' | tr -d '[:space:]')
    if [[ -n "$p" ]]; then
      echo "$p"
      return
    fi
  fi
  echo "7788"
}

port=$(resolve_port)
echo "=> Lune port: $port"

# 按端口杀掉占用进程
pids=$(lsof -ti "tcp:$port" 2>/dev/null || true)
if [[ -n "$pids" ]]; then
  echo "=> Killing processes on port $port: $pids"
  echo "$pids" | xargs kill 2>/dev/null || true
  sleep 1
fi

# 按命令特征兜底杀
pkill -f "go-build.*lune|/tmp/lune" 2>/dev/null || true

echo "=> Starting lune..."
exec go run ./cmd/lune
