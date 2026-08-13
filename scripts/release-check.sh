#!/usr/bin/env sh
set -eu

(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go test ./...)
test -f internal/RELEASE.md
test -f internal/SBOM.spdx.json
echo "release baseline: PASS"
