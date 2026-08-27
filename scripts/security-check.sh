#!/usr/bin/env sh
set -eu

test -f backend/internal/protocol/keyring.go
test -f backend/internal/worker/store.go
! rg -n 'DATABASE_URL|postgres://' backend/cmd/worker backend/internal/worker
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go vet ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" "${GOVULNCHECK_BIN:-govulncheck}" ./...)
(cd frontend && npm audit --omit=dev --audit-level=low)
echo "security baseline: PASS"
