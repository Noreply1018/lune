#!/usr/bin/env bash
set -euo pipefail

out="${1:-/out/$(date -u +"%Y%m%dT%H%M%SZ")}"
data_dir="${LUNE_DATA_DIR:-/data}"
raw_tmp="$(mktemp -d)"
trap 'rm -rf "$raw_tmp"' EXIT

mkdir -p "$out/redacted"

{
  echo "generated_at=$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  echo "data_dir=$data_dir"
  echo "hostname=$(hostname)"
  echo
  echo "tools:"
  echo "  bash=$(bash --version | head -n 1)"
  echo "  curl=$(curl --version | head -n 1)"
  echo "  jq=$(jq --version)"
  echo "  sqlite3=$(sqlite3 --version)"
} > "$out/summary.txt"

find "$data_dir" -maxdepth 3 -type f ! -path "$data_dir/cpa-auth/*" ! -path "$data_dir/audit-cpa-import/*" -printf '%P\t%s bytes\t%TY-%Tm-%TdT%TH:%TM:%TSZ\n' \
  | sort > "$raw_tmp/files.txt" 2>/dev/null || true

if [[ -d "$data_dir/cpa-auth" ]]; then
  {
    printf 'account_key_hash\tsize_bytes\tmodified_at\n'
    while IFS= read -r -d '' file; do
      base="$(basename "$file")"
      key="${base%.json}"
      key_hash="$(printf '%s' "$key" | sha256sum | awk '{print $1}')"
      size="$(stat -c '%s' "$file")"
      modified_at="$(stat -c '%y' "$file" | sed -E 's/ ([+-][0-9]{4})$//')"
      printf 'sha256:%s\t%s\t%s\n' "$key_hash" "$size" "$modified_at"
    done < <(find "$data_dir/cpa-auth" -maxdepth 1 -type f -name '*.json' -print0 | sort -z)
  } > "$raw_tmp/cpa-auth-files.txt" 2>/dev/null || true
else
  echo "cpa-auth directory not found" > "$raw_tmp/cpa-auth-files.txt"
fi

awk '$2 == "/data" {print}' /proc/mounts > "$raw_tmp/mounts.txt" 2>/dev/null || true

db_file="$(find "$data_dir" -maxdepth 2 -type f -name '*.db' | sort | head -n 1 || true)"
if [[ -n "$db_file" ]]; then
  {
    echo "db_file=$db_file"
    sqlite3 -readonly "$db_file" ".tables"
  } > "$raw_tmp/db-tables.txt" 2>&1 || true

  sqlite3 -readonly "$db_file" <<'SQL' > "$raw_tmp/db-counts.tsv" 2>&1 || true
.headers on
.mode tabs
SELECT 'accounts' AS table_name, COUNT(*) AS row_count FROM accounts
UNION ALL SELECT 'pools', COUNT(*) FROM pools
UNION ALL SELECT 'pool_members', COUNT(*) FROM pool_members
UNION ALL SELECT 'access_tokens', COUNT(*) FROM access_tokens
UNION ALL SELECT 'request_logs', COUNT(*) FROM request_logs;
SQL

  sqlite3 -readonly "$db_file" -json <<'SQL' > "$raw_tmp/recent-operations.json" 2>&1 || true
SELECT operation_id, operation_type, source, target_type, target_id, target_summary, status, error_code, safe_error_message, correlation_id, started_at, finished_at, created_at
FROM operations
ORDER BY datetime(created_at) DESC, id DESC
LIMIT 10;
SQL
  sqlite3 -readonly "$db_file" -json <<'SQL' > "$raw_tmp/recent-operation-items.json" 2>&1 || true
WITH recent AS (
  SELECT operation_id, id
  FROM operations
  ORDER BY datetime(created_at) DESC, id DESC
  LIMIT 10
)
SELECT oi.operation_id, oi.item_index, 'ACCOUNT_KEY_REDACTED.json' AS client_file_name, oi.account_key_hash, oi.action, oi.status, oi.runtime_sync, oi.error_code, oi.safe_error_message, oi.account_id, oi.pool_member_id, oi.stage, oi.created_at
FROM operation_items oi
JOIN recent r ON r.operation_id = oi.operation_id
ORDER BY r.id DESC, oi.item_index, oi.id;
SQL
else
  echo "no sqlite db found under $data_dir" > "$raw_tmp/db-tables.txt"
fi

if [[ -n "$db_file" ]] && command -v lune >/dev/null 2>&1; then
  for cmd in summary recent-operations db-integrity cpa-auth cpa-runtime; do
    lune debug "$cmd" > "$raw_tmp/debug-$cmd.json" 2>&1 || true
  done
  sqlite3 -readonly "$db_file" "SELECT operation_id FROM operations ORDER BY datetime(created_at) DESC, id DESC LIMIT 10;" 2>/dev/null \
    | nl -w2 -s' ' \
    | while read -r index operation_id; do
      [[ -n "$operation_id" ]] || continue
      lune debug operation "$operation_id" > "$raw_tmp/debug-operation-$index.json" 2>&1 || true
    done
  first_account_id="$(sqlite3 -readonly "$db_file" "SELECT id FROM accounts ORDER BY id LIMIT 1;" 2>/dev/null || true)"
  if [[ -n "$first_account_id" ]]; then
    lune debug account "$first_account_id" > "$raw_tmp/debug-account.json" 2>&1 || true
  fi
elif ! command -v lune >/dev/null 2>&1; then
  echo "lune binary not available in audit tools container" > "$raw_tmp/debug-unavailable.txt"
fi

for file in "$raw_tmp"/*; do
  [[ -f "$file" ]] || continue
  /work/scripts/audit/redact.sh "$file" "$out/redacted/$(basename "$file")"
done

echo "tool_collect_status=ok" >> "$out/summary.txt"

cp "$out/summary.txt" "$out/redacted/summary.txt"

tar -C "$out" -czf "$out/lune-audit-redacted.tar.gz" redacted summary.txt
