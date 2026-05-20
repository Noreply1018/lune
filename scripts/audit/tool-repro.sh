#!/usr/bin/env bash
set -euo pipefail

scenario="${1:-}"
out="${2:-/out/$(date -u +"%Y%m%dT%H%M%SZ")}"
data_dir="${LUNE_DATA_DIR:-/data}"
cpa_import_dir="${LUNE_AUDIT_CPA_IMPORT_DIR:-$data_dir/cpa-auth}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

[[ -n "$scenario" ]] || {
  echo "error: scenario is required" >&2
  exit 1
}

mkdir -p "$out/redacted/scenario"

db_file="$(find "$data_dir" -maxdepth 2 -type f -name '*.db' | sort | head -n 1 || true)"
[[ -n "$db_file" ]] || {
  echo "error: no sqlite db found under $data_dir" >&2
  exit 1
}

api_base="${LUNE_API_BASE:-http://127.0.0.1:${LUNE_PORT:-7788}}"

json_escape() {
  jq -Rn --arg value "$1" '$value'
}

admin_token() {
  sqlite3 -readonly "$db_file" "SELECT value FROM system_config WHERE key='admin_token' LIMIT 1;"
}

api_curl() {
  local method="$1"
  local path="$2"
  local token="$3"
  local body="${4:-}"
  if [[ -n "$body" ]]; then
    curl -fsS -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      --data "$body" \
      "$api_base$path"
    return
  fi
  curl -fsS -X "$method" \
    -H "Authorization: Bearer $token" \
    "$api_base$path"
}

write_json() {
  local file="$1"
  shift
  jq -n "$@" > "$out/redacted/scenario/$file"
}

json_get() {
  local file="$1"
  local expr="$2"
  jq -r "$expr" "$file"
}

build_multipart_batch() {
  local pool_id="$1"
  shift
  local manifest="$1"
  shift
  local body_file="$1"
  local boundary="----lune-audit-$(date -u +%s%N)-$$"
  {
    while IFS='|' read -r name path; do
      [[ -n "$name" && -n "$path" ]] || continue
      printf -- '--%s\r\n' "$boundary"
      printf 'Content-Disposition: form-data; name="files"; filename="%s"\r\n' "$name"
      printf 'Content-Type: application/json\r\n\r\n'
      cat "$path"
      printf '\r\n'
    done < "$manifest"
    printf -- '--%s\r\n' "$boundary"
    printf 'Content-Disposition: form-data; name="pool_id"\r\n\r\n%s\r\n' "$pool_id"
    printf -- '--%s--\r\n' "$boundary"
  } > "$body_file"
  printf '%s' "$boundary"
}

admin_request() {
  local method="$1"
  local path="$2"
  local token="$3"
  local body_file="${4:-}"
  local content_type="${5:-}"
  local out_file="${6:-}"
  if [[ -n "$out_file" ]]; then
    if [[ -n "$body_file" ]]; then
      curl -fsS -X "$method" \
        -H "Authorization: Bearer $token" \
        -H "${content_type:-Content-Type: application/json}" \
        --data-binary @"$body_file" \
        -o "$out_file" \
        "$api_base$path"
    else
      curl -fsS -X "$method" \
        -H "Authorization: Bearer $token" \
        -o "$out_file" \
        "$api_base$path"
    fi
    return
  fi
  if [[ -n "$body_file" ]]; then
    curl -fsS -X "$method" \
      -H "Authorization: Bearer $token" \
      -H "${content_type:-Content-Type: application/json}" \
      --data-binary @"$body_file" \
      "$api_base$path"
  else
    curl -fsS -X "$method" \
      -H "Authorization: Bearer $token" \
      "$api_base$path"
  fi
}

stateful_probe() {
  local admin
  admin="$(admin_token)"
  [[ -n "$admin" ]] || {
    echo "error: admin token unavailable from DB" >&2
    exit 1
  }

  local row pool_id account_id model token token_id
  row="$(sqlite3 -readonly "$db_file" <<'SQL'
.mode tabs
SELECT p.id, a.id, COALESCE(am.model_id, 'gpt-test') AS model
FROM pools p
JOIN pool_members pm ON pm.pool_id = p.id AND pm.enabled = 1
JOIN accounts a ON a.id = pm.account_id
LEFT JOIN account_models am ON am.account_id = a.id
WHERE p.enabled = 1
  AND a.enabled = 1
  AND a.status IN ('healthy', 'degraded')
  AND a.serving_status <> 'error'
  AND (a.serving_status <> 'cooldown' OR (a.cooldown_until <> '' AND datetime(a.cooldown_until) <= datetime('now')))
  AND NOT EXISTS (
    SELECT 1 FROM account_diagnostics ad
    WHERE ad.account_id = a.id
      AND (
        lower(trim(ad.scheduler_status)) NOT IN ('eligible', 'eligible_with_warning')
        OR (
          trim(COALESCE(ad.scheduler_override, '')) = ''
          AND lower(trim(ad.stable_diagnostic_status)) IN ('banned', 'quota_exhausted', 'auth_invalid')
        )
      )
  )
ORDER BY p.id, pm.position, a.id
LIMIT 1;
SQL
)"
  [[ -n "$row" ]] || {
    echo "error: no routable account available for stateful-probe scenario" >&2
    exit 1
  }
  IFS=$'\t' read -r pool_id account_id model <<< "$row"
  [[ -n "$pool_id" && -n "$account_id" ]] || {
    echo "error: failed to resolve pool/account for stateful-probe" >&2
    exit 1
  }

  token_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM access_tokens WHERE pool_id=$pool_id ORDER BY id LIMIT 1;")"
  if [[ -z "$token_id" ]]; then
    api_curl GET "/admin/api/pools/$pool_id/tokens" "$admin" >/dev/null
    token_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM access_tokens WHERE pool_id=$pool_id ORDER BY id LIMIT 1;")"
  fi
  [[ -n "$token_id" ]] || {
    echo "error: no pool token available for pool $pool_id" >&2
    exit 1
  }
  token="$(api_curl POST "/admin/api/tokens/$token_id/reveal" "$admin" | jq -r '.data.token // empty')"
  [[ -n "$token" ]] || {
    echo "error: failed to reveal pool token $token_id" >&2
    exit 1
  }

  local before_id body http_code response_file response_bytes
  before_id="$(sqlite3 -readonly "$db_file" "SELECT COALESCE(MAX(id), 0) FROM request_logs;")"
  body="$(jq -n --arg model "$model" '{model:$model, messages:[{role:"user", content:"lune audit stateful probe"}]}')"
  response_file="$tmp_dir/stateful-probe-response.raw"
  http_code="$(curl -sS -o "$response_file" -w '%{http_code}' \
    -H "Authorization: Bearer $token" \
    -H "X-Lune-Account-Id: $account_id" \
    -H "X-Lune-Probe-Mode: stateful" \
    -H "Content-Type: application/json" \
    --data "$body" \
    "$api_base/v1/chat/completions")"
  response_bytes="$(wc -c < "$response_file" | tr -d ' ')"

  local found
  found=""
  for _ in $(seq 1 30); do
    found="$(sqlite3 -readonly "$db_file" "SELECT id FROM request_logs WHERE id > $before_id AND account_id=$account_id AND pool_id=$pool_id AND force_account=1 AND stateful_probe=1 AND traffic_kind='stateful_probe' ORDER BY id DESC LIMIT 1;")"
    [[ -n "$found" ]] && break
    sleep 1
  done
  [[ -n "$found" ]] || {
    echo "error: stateful probe request log was not recorded" >&2
    exit 1
  }

  sqlite3 -readonly "$db_file" -json \
    "SELECT id, request_id, pool_id, account_id, status_code, success, diagnostic, force_account, stateful_probe, traffic_kind, model_requested, source_kind, created_at FROM request_logs WHERE id=$found;" \
    > "$out/redacted/scenario/stateful-probe-log.json"

  jq -n \
    --arg scenario "stateful-probe" \
    --arg status "ok" \
    --arg http_code "$http_code" \
    --arg pool_id "$pool_id" \
    --arg account_id "$account_id" \
    --arg model "$model" \
    --arg request_log_id "$found" \
    --arg response_bytes "$response_bytes" \
    '{scenario:$scenario,status:$status,http_code:($http_code|tonumber),response_bytes:($response_bytes|tonumber),pool_id:($pool_id|tonumber),account_id:($account_id|tonumber),model:$model,request_log_id:($request_log_id|tonumber)}' \
    > "$out/redacted/scenario/stateful-probe-summary.json"
}

cpa_import_reimport() {
  local admin pool_id token batch_body boundary manifest batch_resp account_id delete_resp latest_op target_name target_key target_hash
  local before_delete_count post_delete_count reimport_account_id post_reimport_count import_count cpa_accounts cpa_files reimport_candidate batch_id reimport_batch_id token_id
  admin="$(admin_token)"
  [[ -n "$admin" ]] || {
    echo "error: admin token unavailable from DB" >&2
    exit 1
  }

  pool_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM pools WHERE enabled=1 ORDER BY priority DESC, id ASC LIMIT 1;")"
  [[ -n "$pool_id" ]] || {
    echo "error: no enabled pool available for cpa-import-reimport" >&2
    exit 1
  }
  token_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM access_tokens WHERE pool_id=$pool_id ORDER BY id LIMIT 1;")"
  if [[ -z "$token_id" ]]; then
    admin_request GET "/admin/api/pools/$pool_id/tokens" "$admin" >/dev/null
    token_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM access_tokens WHERE pool_id=$pool_id ORDER BY id LIMIT 1;")"
  fi
  [[ -n "$token_id" ]] || {
    echo "error: no pool token available for pool $pool_id" >&2
    exit 1
  }
  token="$(admin_request POST "/admin/api/tokens/$token_id/reveal" "$admin" "" "" | jq -r '.data.token // empty')"
  [[ -n "$token" ]] || {
    echo "error: failed to reveal pool token $token_id" >&2
    exit 1
  }

  manifest="$tmp_dir/cpa-manifest.txt"
  : > "$manifest"
  if [[ -d "$cpa_import_dir" ]]; then
    while IFS= read -r -d '' file; do
      base="$(basename "$file")"
      printf '%s|%s\n' "$base" "$file" >> "$manifest"
    done < <(find "$cpa_import_dir" -maxdepth 1 -type f -name '*.json' ! -name '.login-sessions.json' -print0 | sort -z)
  fi
  [[ -s "$manifest" ]] || {
    echo "error: no CPA auth JSON files available under $cpa_import_dir" >&2
    exit 1
  }
  target_name="$(head -n 1 "$manifest" | cut -d'|' -f1)"
  target_key="${target_name%.json}"
  target_hash="$(printf '%s' "$target_key" | sha256sum | awk '{print $1}')"

  batch_body="$tmp_dir/cpa-batch.bin"
  boundary="$(build_multipart_batch "$pool_id" "$manifest" "$batch_body")"
  batch_resp="$tmp_dir/import-response.json"
  admin_request POST "/admin/api/accounts/cpa/import-json-batch" "$admin" "$batch_body" "Content-Type: multipart/form-data; boundary=$boundary" "$batch_resp"
  jq '{data:{batch_id:.data.batch_id,pool_id:.data.pool_id,summary:.data.summary,items:((.data.items // []) | map({client_file_name:"ACCOUNT_KEY_REDACTED.json",account_key_hash,account_id_suffix,account_id,action,status,runtime_sync,error_code,error_message,runtime_error_code,runtime_error_message,stage}))}}' \
    "$batch_resp" > "$out/redacted/scenario/cpa-import-reimport-import-response.json"

  batch_id="$(jq -r '.data.batch_id // empty' "$batch_resp")"
  [[ -n "$batch_id" ]] || {
    echo "error: import batch did not return batch_id" >&2
    exit 1
  }
  account_id="$(jq -r --arg file "$target_name" '.data.items[] | select(.client_file_name == $file and (.status == "created" or .status == "updated") and (.account_id // 0) > 0) | .account_id' "$batch_resp" | head -n 1)"
  [[ -n "$account_id" ]] || {
    echo "error: import batch did not return a CPA account id" >&2
    exit 1
  }
  before_delete_count="$(sqlite3 -readonly "$db_file" "SELECT COUNT(*) FROM accounts WHERE id=$account_id AND source_kind='cpa';")"
  [[ "$before_delete_count" -ge 1 ]] || {
    echo "error: imported CPA account is missing before delete" >&2
    exit 1
  }

  delete_resp="$tmp_dir/delete-response.json"
  admin_request DELETE "/admin/api/accounts/$account_id" "$admin" "" "" "$delete_resp"
  cp "$delete_resp" "$out/redacted/scenario/cpa-import-reimport-delete-response.json"

  sqlite3 -readonly "$db_file" -json <<SQL > "$out/redacted/scenario/cpa-import-reimport-delete-log.json"
SELECT id, request_id, pool_id, account_id, status_code, success, source_kind, diagnostic, force_account, stateful_probe, traffic_kind, runtime_binding_status, runtime_binding_reason, created_at
FROM request_logs
WHERE account_id = $account_id
ORDER BY id DESC
LIMIT 5;
SQL

  post_delete_count="$(sqlite3 -readonly "$db_file" "SELECT COUNT(*) FROM accounts WHERE id=$account_id;")"
  [[ "$post_delete_count" -eq 0 ]] || {
    echo "error: CPA account still exists after delete" >&2
    exit 1
  }

  batch_resp="$tmp_dir/reimport-response.json"
  admin_request POST "/admin/api/accounts/cpa/import-json-batch" "$admin" "$batch_body" "Content-Type: multipart/form-data; boundary=$boundary" "$batch_resp"
  jq '{data:{batch_id:.data.batch_id,pool_id:.data.pool_id,summary:.data.summary,items:((.data.items // []) | map({client_file_name:"ACCOUNT_KEY_REDACTED.json",account_key_hash,account_id_suffix,account_id,action,status,runtime_sync,error_code,error_message,runtime_error_code,runtime_error_message,stage}))}}' \
    "$batch_resp" > "$out/redacted/scenario/cpa-import-reimport-reimport-response.json"
  reimport_batch_id="$(jq -r '.data.batch_id // empty' "$batch_resp")"
  [[ -n "$reimport_batch_id" ]] || {
    echo "error: reimport batch did not return batch_id" >&2
    exit 1
  }
  reimport_account_id="$(jq -r --arg file "$target_name" '.data.items[] | select(.client_file_name == $file and (.status == "created" or .status == "updated") and (.account_id // 0) > 0) | .account_id' "$batch_resp" | head -n 1)"
  [[ -n "$reimport_account_id" ]] || {
    echo "error: reimport batch did not return the target CPA account id" >&2
    exit 1
  }
  post_reimport_count="$(sqlite3 -readonly "$db_file" "SELECT COUNT(*) FROM accounts WHERE id=$reimport_account_id AND source_kind='cpa';")"
  [[ "$post_reimport_count" -ge 1 ]] || {
    echo "error: reimported CPA account is missing" >&2
    exit 1
  }

  sqlite3 -readonly "$db_file" -json <<SQL > "$out/redacted/scenario/cpa-import-reimport-operations.json"
SELECT operation_id, operation_type, source, target_type, target_id, target_summary, status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
FROM operations
WHERE operation_id IN ('$batch_id', '$reimport_batch_id')
ORDER BY datetime(created_at) ASC, id ASC;
SQL
  sqlite3 -readonly "$db_file" -json <<SQL > "$out/redacted/scenario/cpa-import-reimport-items.json"
SELECT oi.operation_id, oi.item_index, 'ACCOUNT_KEY_REDACTED.json' AS client_file_name, oi.account_key_hash, oi.action, oi.status, oi.runtime_sync, oi.error_code, oi.safe_error_message, oi.account_id, oi.pool_member_id, oi.stage, oi.created_at
FROM operation_items oi
WHERE oi.operation_id IN ('$batch_id', '$reimport_batch_id')
ORDER BY oi.operation_id, oi.item_index, oi.id;
SQL
  sqlite3 -readonly "$db_file" -json <<SQL > "$out/redacted/scenario/cpa-import-reimport-account.json"
SELECT id, source_kind, cpa_service_id, cpa_provider, cpa_plan_type, cpa_disabled, cpa_credential_status, cpa_subscription_status, cpa_access_status, cpa_quota_status, serving_status, enabled, status, created_at, updated_at
FROM accounts
WHERE id = $reimport_account_id AND source_kind='cpa'
ORDER BY id DESC
LIMIT 1;
SQL

  latest_op="$(sqlite3 -readonly "$db_file" "SELECT operation_id FROM operations ORDER BY datetime(created_at) DESC, id DESC LIMIT 1;")"
  cpa_accounts="$(sqlite3 -readonly "$db_file" "SELECT COUNT(*) FROM accounts WHERE source_kind='cpa';")"
  cpa_files="$(find "$data_dir/cpa-auth" -maxdepth 1 -type f -name '*.json' ! -name '.login-sessions.json' | wc -l | tr -d ' ')"
  import_count="$(sqlite3 -readonly "$db_file" "SELECT COUNT(*) FROM operations WHERE operation_type='cpa_import_batch';")"
  reimport_candidate="$(sqlite3 -readonly "$db_file" <<'SQL'
SELECT COUNT(*)
FROM operation_items
WHERE action IN ('updated', 'skipped')
   OR error_code IN ('duplicate_in_batch', 'identity_mismatch');
SQL
)"

  jq -n \
    --arg scenario "cpa-import-reimport" \
    --arg status "ok" \
    --arg pool_id "$pool_id" \
    --arg batch_id "$batch_id" \
    --arg reimport_batch_id "$reimport_batch_id" \
    --arg latest_operation_id "$latest_op" \
    --arg account_id "$account_id" \
    --arg reimport_account_id "$reimport_account_id" \
    --arg target_name "$target_name" \
    --arg target_key_hash "$target_hash" \
    --argjson import_count "$import_count" \
    --argjson cpa_accounts "$cpa_accounts" \
    --argjson cpa_files "$cpa_files" \
    --argjson reimport_candidate "$reimport_candidate" \
    '{scenario:$scenario,status:$status,pool_id:($pool_id|tonumber),target_auth_file:"ACCOUNT_KEY_REDACTED.json",target_account_key_hash:$target_key_hash,initial_batch_id:$batch_id,reimport_batch_id:$reimport_batch_id,latest_operation_id:$latest_operation_id,deleted_account_id:($account_id|tonumber),reimported_account_id:($reimport_account_id|tonumber),import_operations:$import_count,cpa_accounts:$cpa_accounts,cpa_auth_files:$cpa_files,reimport_evidence_items:$reimport_candidate}' \
    > "$out/redacted/scenario/cpa-import-reimport-summary.json"
}

case "$scenario" in
  stateful-probe)
    stateful_probe
    ;;
  cpa-import-reimport)
    cpa_import_reimport
    ;;
  *)
    echo "error: scenario_not_implemented: $scenario" >&2
    exit 1
    ;;
esac
