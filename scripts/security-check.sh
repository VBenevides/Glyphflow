#!/usr/bin/env sh
set -eu

test -f internal/v0-review/REPORT.md
test -f backend/internal/protocol/keyring.go
test -f backend/internal/worker/store.go
! rg -n 'DATABASE_URL|postgres://' backend/cmd/worker backend/internal/worker
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go vet ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" "${GOVULNCHECK_BIN:-govulncheck}" ./...)
(cd frontend && npm audit --omit=dev --audit-level=low)
test -s internal/v0-review/SBOM.spdx.json
! rg -n '"packages"[[:space:]]*:[[:space:]]*\[\]' internal/v0-review/SBOM.spdx.json
echo "security baseline: PASS"
