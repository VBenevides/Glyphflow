#!/usr/bin/env sh
set -eu

(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go vet ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go mod verify)
release_tmp=$(mktemp -d)
trap 'rm -rf "$release_tmp"' EXIT
./scripts/generate-sbom.sh "$release_tmp/SBOM.spdx.json"
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go build -o "$release_tmp/controlplane" ./cmd/controlplane)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go build -o "$release_tmp/worker" ./cmd/worker)
test -s "$release_tmp/SBOM.spdx.json"
! rg -n '"packages"[[:space:]]*:[[:space:]]*\[\]' "$release_tmp/SBOM.spdx.json"
test -s "$release_tmp/controlplane"
test -s "$release_tmp/worker"
echo "release baseline: PASS"
