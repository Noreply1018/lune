#!/usr/bin/env sh
set -eu

yaml_quote() {
  printf "'"
  printf "%s" "$1" | sed "s/'/''/g"
  printf "'"
}

stop_children() {
  if [ -n "${LUNE_PID:-}" ]; then
    kill "$LUNE_PID" 2>/dev/null || true
  fi
  if [ -n "${CPA_PID:-}" ]; then
    kill "$CPA_PID" 2>/dev/null || true
  fi
}

run_embedded_cpa() {
  : "${CPA_PORT:=8317}"
  : "${CPA_API_KEY:=sk-cpa-default}"
  : "${LUNE_CPA_AUTH_DIR:=/app/data/cpa-auth}"
  : "${LUNE_CPA_MANAGEMENT_KEY:=lune-cpa-management-dev}"
  : "${LUNE_CPA_API_KEY:=$CPA_API_KEY}"
  : "${LUNE_CPA_BASE_URL:=http://127.0.0.1:${CPA_PORT}}"

  export CPA_API_KEY
  export LUNE_CPA_AUTH_DIR
  export LUNE_CPA_MANAGEMENT_KEY
  export LUNE_CPA_API_KEY
  export LUNE_CPA_BASE_URL

  mkdir -p "$LUNE_CPA_AUTH_DIR" /CLIProxyAPI

  {
    printf "port: %s\n" "$CPA_PORT"
    printf "auth-dir: %s\n" "$(yaml_quote "$LUNE_CPA_AUTH_DIR")"
    printf "remote-management:\n"
    printf "  allow-remote: true\n"
    printf "  secret-key: %s\n" "$(yaml_quote "$LUNE_CPA_MANAGEMENT_KEY")"
    printf "api-keys:\n"
    printf "  - %s\n" "$(yaml_quote "$LUNE_CPA_API_KEY")"
    printf "debug: false\n"
  } > /CLIProxyAPI/config.yaml

  (
    cd /CLIProxyAPI
    ./CLIProxyAPI 2>&1 | sed -u 's/^/[cpa] /'
  ) &
  CPA_PID="$!"
}

cmd="${1:-up}"

case "$cmd" in
  up)
    : "${LUNE_PORT:=7788}"
    : "${LUNE_DATA_DIR:=/app/data}"
    : "${LUNE_GATEWAY_TMP_DIR:=${LUNE_DATA_DIR}/tmp}"
    : "${LUNE_EMBEDDED_CPA:=1}"
    export LUNE_PORT LUNE_DATA_DIR LUNE_GATEWAY_TMP_DIR LUNE_EMBEDDED_CPA

    mkdir -p "$LUNE_DATA_DIR" "$LUNE_GATEWAY_TMP_DIR"

    if [ "$LUNE_EMBEDDED_CPA" != "0" ]; then
      run_embedded_cpa
    fi

    trap stop_children INT TERM

    lune up &
    LUNE_PID="$!"

    set +e
    wait "$LUNE_PID"
    status="$?"
    set -e
    stop_children
    wait ${CPA_PID:-} 2>/dev/null || true
    exit "$status"
    ;;
  check|version)
    exec lune "$@"
    ;;
  lune)
    shift
    exec lune "$@"
    ;;
  *)
    exec "$@"
    ;;
esac
