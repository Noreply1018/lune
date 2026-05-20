#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -ne 2 ]]; then
  echo "Usage: scripts/audit/redact.sh <input> <output>" >&2
  exit 1
fi

input="$1"
output="$2"

if [[ ! -f "$input" ]]; then
  echo "error: input file not found: $input" >&2
  exit 1
fi

mkdir -p "$(dirname "$output")"

sed -E \
  -e 's/sk-[A-Za-z0-9_-]{8,}/sk-REDACTED/g' \
  -e 's#(^|[[:space:]])((/[^[:space:]]*)?cpa-auth/)[^/[:space:]]+\.json#\1\2ACCOUNT_KEY_REDACTED.json#g' \
  -e 's#(^|[[:space:]])[^/[:space:]]+@[^/[:space:]]+\.json#\1ACCOUNT_KEY_REDACTED.json#g' \
  -e 's#(^|[[:space:]])([A-Za-z0-9._%+-]{4,}[-_][A-Za-z0-9._%+-]{4,}\.json)([[:space:]]|$)#\1ACCOUNT_KEY_REDACTED.json\3#g' \
  -e 's#(^|[[:space:]])[^/[:space:]]+\.json([[:space:]]|$)#\1ACCOUNT_KEY_REDACTED.json\2#g' \
  -e 's/(Authorization:[[:space:]]*Bearer[[:space:]]+)[^[:space:]]+/\1REDACTED/gI' \
  -e 's/(x-api-key:[[:space:]]*)[^[:space:]]+/\1REDACTED/gI' \
  -e 's/(refresh_token|refreshToken|access_token|accessToken|id_token|idToken|api_key|apiKey|pool_token|poolToken|management_key|managementKey|account_key|accountKey|token)"[[:space:]]*:[[:space:]]*"[^"]*"/\1":"REDACTED"/gI' \
  -e 's/(refresh_token|refreshToken|access_token|accessToken|id_token|idToken|api_key|apiKey|pool_token|poolToken|management_key|managementKey|account_key|accountKey|token)=[^[:space:]&]+/\1=REDACTED/gI' \
  -e 's/([?&](refresh_token|refreshToken|access_token|accessToken|id_token|idToken|api_key|apiKey|pool_token|poolToken|management_key|managementKey|account_key|accountKey|token)=)[^[:space:]&]+/\1REDACTED/gI' \
  -e 's/[A-Za-z0-9._%+-]{2}[A-Za-z0-9._%+-]*@([A-Za-z0-9.-]+\.[A-Za-z]{2,})/***@\1/g' \
  "$input" > "$output"
