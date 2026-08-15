#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output_dir="${RUNNER_BINARIES_DIR:-$project_root/runner-binaries}"
export GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}"
mkdir -p "$output_dir"

build_headless() {
  local goos="$1" output="$2"
  (cd "$project_root" && GOOS="$goos" GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$output" ./cmd/worker)
}

build_desktop() {
  local goos="$1" output="$2"
  (cd "$project_root" && GOOS="$goos" GOARCH=amd64 go build -tags workerui -trimpath -ldflags='-s -w' -o "$output" ./cmd/worker)
}

build_desktop linux "$output_dir/glyphflow-runner-linux-amd64"
build_desktop windows "$output_dir/glyphflow-runner-windows-amd64.exe"
build_headless linux "$output_dir/glyphflow-runner-linux-amd64-headless"
build_headless windows "$output_dir/glyphflow-runner-windows-amd64-headless.exe"
