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

find "$data_dir" -maxdepth 3 -type f ! -path "$data_dir/cpa-auth/*" -printf '%P\t%s bytes\t%TY-%Tm-%TdT%TH:%TM:%TSZ\n' \
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
else
  echo "no sqlite db found under $data_dir" > "$raw_tmp/db-tables.txt"
fi

for file in "$raw_tmp"/*; do
  [[ -f "$file" ]] || continue
  /work/scripts/audit/redact.sh "$file" "$out/redacted/$(basename "$file")"
done

echo "tool_collect_status=ok" >> "$out/summary.txt"

cp "$out/summary.txt" "$out/redacted/summary.txt"

tar -C "$out" -czf "$out/lune-audit-redacted.tar.gz" redacted summary.txt
