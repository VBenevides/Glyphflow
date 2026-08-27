#!/usr/bin/env sh
set -eu

test -f backend/internal/protocol/keyring.go
test -f backend/internal/worker/store.go
! grep -R -n -E 'DATABASE_URL|postgres://' backend/cmd/worker backend/internal/worker
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" go vet ./...)
govulncheck_bin=${GOVULNCHECK_BIN:-govulncheck}
command -v "$govulncheck_bin" >/dev/null 2>&1 || { echo "security check: govulncheck is required" >&2; exit 1; }
for tags in default workerui workerui_tui; do
  if [ "$tags" = default ]; then
    (cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" "$govulncheck_bin" ./...)
  else
    (cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache-security}" "$govulncheck_bin" -tags "$tags" ./...)
  fi
done
(cd frontend && npm audit --omit=dev --audit-level=low)
echo "security baseline: PASS"
