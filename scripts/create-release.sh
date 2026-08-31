#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
release_version=$(tr -d '[:space:]' < "$repo_root/VERSION")
release_notes=$(mktemp)

cleanup() {
  rm -f "$release_notes"
}
trap cleanup EXIT INT TERM

command -v gh >/dev/null 2>&1 || {
  echo "release: gh is required" >&2
  exit 1
}
[ -n "$release_version" ] || {
  echo "release: VERSION is empty" >&2
  exit 1
}

awk -v version="$release_version" '
  index($0, "## [" version "] - ") == 1 { found=1; next }
  found && /^## \[/ { exit }
  found { print }
' "$repo_root/CHANGELOG.md" > "$release_notes"

[ -s "$release_notes" ] || {
  echo "release: no CHANGELOG.md section found for $release_version" >&2
  exit 1
}

gh release create "v$release_version" \
  --title "Glyphflow v$release_version" \
  --notes-file "$release_notes"
