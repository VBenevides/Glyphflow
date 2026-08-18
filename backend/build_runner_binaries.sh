#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output_dir="${RUNNER_BINARIES_DIR:-$project_root/runner-binaries}"
export GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}"
mkdir -p "$output_dir"
version="$(tr -d '[:space:]' < "$project_root/../VERSION")"
[[ -n "$version" ]] || { echo "VERSION is empty" >&2; exit 1; }
base_ldflags="-s -w -X github.com/VBenevides/Glyphflow/backend.Version=$version"

build_headless() {
  local goos="$1" output="$2"
  (cd "$project_root" && GOOS="$goos" GOARCH=amd64 go build -trimpath -ldflags="$base_ldflags" -o "$output" ./cmd/worker)
}

build_desktop() {
  local goos="$1" output="$2"
  local ldflags="$base_ldflags"
  if [[ "$goos" == windows ]]; then
    ldflags+=' -H=windowsgui'
  fi
  (cd "$project_root" && GOOS="$goos" GOARCH=amd64 go build -tags workerui -trimpath -ldflags="$ldflags" -o "$output" ./cmd/worker)
}

build_tui() {
  local goos="$1" output="$2"
  (cd "$project_root" && GOOS="$goos" GOARCH=amd64 go build -tags workerui_tui -trimpath -ldflags="$base_ldflags" -o "$output" ./cmd/worker)
}

build_desktop linux "$output_dir/glyphflow-runner-linux-amd64"
build_desktop windows "$output_dir/glyphflow-runner-windows-amd64.exe"
build_tui linux "$output_dir/glyphflow-runner-linux-amd64-tui"
build_tui windows "$output_dir/glyphflow-runner-windows-amd64-tui.exe"
build_headless linux "$output_dir/glyphflow-runner-linux-amd64-headless"
build_headless windows "$output_dir/glyphflow-runner-windows-amd64-headless.exe"
