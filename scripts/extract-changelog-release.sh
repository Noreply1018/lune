#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <tag> [output-file]" >&2
  echo "Example: $0 v0.1.8 /tmp/release-notes.md" >&2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
  exit 2
fi

tag="$1"
if [[ "$tag" != v* || "$tag" == "v" ]]; then
  echo "error: release tag must start with v, got: $tag" >&2
  exit 1
fi

version="${tag#v}"
changelog="${CHANGELOG_FILE:-CHANGELOG.md}"

if [[ ! -f "$changelog" ]]; then
  echo "error: changelog file not found: $changelog" >&2
  exit 1
fi

if [[ $# -eq 2 ]]; then
  output_file="$2"
else
  output_dir="${RUNNER_TEMP:-$(mktemp -d)}"
  output_file="$output_dir/release-notes-$version.md"
fi

mkdir -p "$(dirname "$output_file")"

awk -v version="$version" '
  BEGIN {
    target = "^## \\[" version "\\] - [0-9]{4}-[0-9]{2}-[0-9]{2}[[:space:]]*$"
    in_section = 0
    found = 0
  }
  $0 ~ target {
    in_section = 1
    found = 1
    next
  }
  in_section && /^## \[/ {
    exit
  }
  in_section {
    print
  }
  END {
    if (!found) {
      exit 3
    }
  }
' "$changelog" > "$output_file" || {
  status=$?
  if [[ "$status" -eq 3 ]]; then
    echo "error: CHANGELOG.md does not contain a release section for [$version]" >&2
  else
    echo "error: failed to extract release notes from CHANGELOG.md" >&2
  fi
  exit 1
}

# Trim leading and trailing blank lines while keeping the original Markdown body.
perl -0pi -e 's/\A[ \t\r\n]+//; s/[ \t\r\n]+\z/\n/' "$output_file"

if [[ ! -s "$output_file" ]]; then
  echo "error: CHANGELOG.md release section [$version] is empty" >&2
  exit 1
fi

if ! grep -q '[^[:space:]]' "$output_file"; then
  echo "error: CHANGELOG.md release section [$version] has no meaningful content" >&2
  exit 1
fi

blocked_pattern='未发布|发布流程进行中|待完成|后续再完成|以后再处理|TODO|FIXME'
if grep -En "$blocked_pattern" "$output_file" >&2; then
  echo "error: CHANGELOG.md release section [$version] still contains placeholder or unfinished release text" >&2
  exit 1
fi

if [[ "$(tr -d '[:space:]' < "$output_file")" == "暂无。" ]]; then
  echo "error: CHANGELOG.md release section [$version] still contains placeholder text: 暂无。" >&2
  exit 1
fi

echo "release notes extracted to $output_file"

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  echo "release_body=$output_file" >> "$GITHUB_OUTPUT"
fi
