#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_docker.sh"

docker compose up -d --remove-orphans
print_access_hint
