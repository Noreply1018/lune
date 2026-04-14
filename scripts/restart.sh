#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_docker.sh"

target="${1:-lune}"

case "$target" in
  lune)
    docker compose restart lune
    print_access_hint
    ;;
  cpa)
    docker compose restart cpa
    ;;
  all)
    docker compose restart
    print_access_hint
    ;;
  *)
    echo "Usage: ./scripts/restart.sh [lune|cpa|all]" >&2
    exit 1
    ;;
esac
