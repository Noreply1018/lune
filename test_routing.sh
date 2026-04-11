#!/usr/bin/env bash
set -euo pipefail

echo "=== 出口路由验证 ==="
echo "通过 SOCKS5 代理请求 ipinfo.io ..."
echo

RESULT=$(docker compose exec upstream-node \
  curl -sS -x socks5://14ab64b73c90c:643153285e@192.208.4.212:12324 \
  https://ipinfo.io/json)

IP=$(echo "$RESULT" | grep -o '"ip": *"[^"]*"' | head -1 | cut -d'"' -f4)
COUNTRY=$(echo "$RESULT" | grep -o '"country": *"[^"]*"' | head -1 | cut -d'"' -f4)

echo "出口 IP:  $IP"
echo "所在国家: $COUNTRY"
echo
echo "=== 完成 ==="
