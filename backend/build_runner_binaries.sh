#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
output_dir="${RUNNER_BINARIES_DIR:-$project_root/runner-binaries}"
export GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}"
mkdir -p "$output_dir"

(
  cd "$project_root"
  GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$output_dir/glyphflow-runner-linux-amd64" ./cmd/worker
  GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o "$output_dir/glyphflow-runner-windows-amd64.exe" ./cmd/worker
)
