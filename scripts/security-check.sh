#!/usr/bin/env sh
set -eu

test -f internal/SECURITY.md
test -f backend/internal/protocol/keyring.go
test -f backend/internal/worker/store.go
! rg -n 'DATABASE_URL|postgres://' backend/cmd/worker backend/internal/worker
rg -n 'Redact|AllowedSubject|AllowedPath' backend/internal/platform/security.go
echo "security baseline: PASS"
