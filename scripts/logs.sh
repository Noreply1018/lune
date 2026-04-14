#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_docker.sh"

target="${1:-lune}"

case "$target" in
  lune)
    docker compose logs -f lune
    ;;
  cpa)
    docker compose logs -f cpa
    ;;
  all)
    docker compose logs -f
    ;;
  *)
    echo "Usage: ./scripts/logs.sh [lune|cpa|all]" >&2
    exit 1
    ;;
esac
