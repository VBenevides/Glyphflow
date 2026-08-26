#!/usr/bin/env sh
set -eu

release_tmp=$(mktemp -d)
release_db_name="glyphflow_release_$$"
release_database_url=${RELEASE_DATABASE_URL:-}
release_db_owned=false
release_db_with_docker=false

cleanup() {
  if [ "$release_db_owned" = true ]; then
    if [ "$release_db_with_docker" = true ]; then
      docker compose exec -T postgres dropdb --if-exists -U glyphflow "$release_db_name" >/dev/null 2>&1 || true
    else
      PGPASSWORD="${PGPASSWORD:-glyphflow}" dropdb -h localhost --if-exists -U glyphflow "$release_db_name" >/dev/null 2>&1 || true
    fi
  fi
  rm -rf "$release_tmp"
}
trap cleanup EXIT

(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go test ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go vet ./...)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go mod verify)
govulncheck_bin=${GOVULNCHECK_BIN:-govulncheck}
command -v "$govulncheck_bin" >/dev/null 2>&1 || { echo "release check: govulncheck is required" >&2; exit 1; }
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" "$govulncheck_bin" ./...)
(cd frontend && npm audit --omit=dev --audit-level=low)

if [ -z "$release_database_url" ]; then
  if command -v pg_isready >/dev/null 2>&1 && pg_isready -h localhost -p 5432 -U glyphflow >/dev/null 2>&1; then
    command -v createdb >/dev/null 2>&1 || { echo "release check: createdb is required for the local PostgreSQL service" >&2; exit 1; }
    PGPASSWORD="${PGPASSWORD:-glyphflow}" dropdb -h localhost --if-exists -U glyphflow "$release_db_name" >/dev/null
    PGPASSWORD="${PGPASSWORD:-glyphflow}" createdb -h localhost -U glyphflow "$release_db_name"
  else
    command -v docker >/dev/null 2>&1 || { echo "release check: set RELEASE_DATABASE_URL or install Docker" >&2; exit 1; }
    docker compose up -d postgres >/dev/null
    ready=false
    attempt=0
    while [ "$attempt" -lt 30 ]; do
      if docker compose exec -T postgres pg_isready -U glyphflow >/dev/null 2>&1; then
        ready=true
        break
      fi
      attempt=$((attempt + 1))
      sleep 1
    done
    [ "$ready" = true ] || { echo "release check: PostgreSQL did not become ready" >&2; exit 1; }
    docker compose exec -T postgres dropdb --if-exists -U glyphflow "$release_db_name" >/dev/null
    docker compose exec -T postgres createdb -U glyphflow "$release_db_name"
    release_db_with_docker=true
  fi
  release_database_url="postgres://glyphflow:glyphflow@localhost:5432/${release_db_name}?sslmode=disable"
  release_db_owned=true
fi

for migration in backend/migrations/*.sql; do
  if [ "$release_db_with_docker" = true ]; then
    docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U glyphflow -d "$release_db_name" < "$migration" >/dev/null
  else
    command -v psql >/dev/null 2>&1 || { echo "release check: psql is required with RELEASE_DATABASE_URL" >&2; exit 1; }
    psql "$release_database_url" -v ON_ERROR_STOP=1 -f "$migration" >/dev/null
  fi
done
(cd backend && DATABASE_URL="$release_database_url" GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go test ./...)

./scripts/generate-sbom.sh "$release_tmp/SBOM.spdx.json"
release_version=$(tr -d '[:space:]' < VERSION)
[ -n "$release_version" ] || { echo "release check: VERSION is empty" >&2; exit 1; }
release_ldflags="-s -w -X github.com/VBenevides/Glyphflow/backend.Version=$release_version"
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go build -trimpath -ldflags="$release_ldflags" -o "$release_tmp/controlplane" ./cmd/controlplane)
(cd backend && GOCACHE="${GOCACHE:-/tmp/glyphflow-go-cache}" go build -trimpath -ldflags="$release_ldflags" -o "$release_tmp/worker" ./cmd/worker)
test -s "$release_tmp/SBOM.spdx.json"
! rg -n '"packages"[[:space:]]*:[[:space:]]*\[\]' "$release_tmp/SBOM.spdx.json"
test -s "$release_tmp/controlplane"
test -s "$release_tmp/worker"
echo "release baseline: PASS"
