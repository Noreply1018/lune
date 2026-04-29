#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_docker.sh"

target="${1:-lune}"

case "$target" in
  lune|all)
    docker compose restart lune
    print_access_hint
    ;;
  *)
    echo "Usage: ./scripts/restart.sh [lune|all]" >&2
    exit 1
    ;;
esac
