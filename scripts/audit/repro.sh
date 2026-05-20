#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "$0")" && pwd)/_common.sh"

scenario="${1:-}"
[[ -n "$scenario" ]] || die "Usage: scripts/audit/repro.sh <scenario>"

case "$scenario" in
  cpa-import-reimport)
    "$AUDIT_SCRIPT_DIR/collect.sh"
    ;;
  stateful-probe)
    "$AUDIT_SCRIPT_DIR/collect.sh"
    ;;
  runtime-reload|pool-delete|quota-refresh|routing-failover)
    die "scenario_not_implemented: $scenario requires isolated fixture data and API choreography before it can be run safely"
    ;;
  *)
    die "unknown scenario: $scenario"
    ;;
esac
