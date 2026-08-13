#!/usr/bin/env sh
set -eu

(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go vet ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go mod verify)
test -f internal/v0-review/REPORT.md
test -s internal/v0-review/SBOM.spdx.json
! rg -n '"packages"[[:space:]]*:[[:space:]]*\[\]' internal/v0-review/SBOM.spdx.json
echo "release baseline: PASS"
