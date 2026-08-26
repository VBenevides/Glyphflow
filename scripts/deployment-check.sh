#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
image=${1:?usage: $0 APPLICATION_IMAGE}
nats_image=${NATS_IMAGE_REF:-nats:2.10-alpine}
postgres_image=${POSTGRES_IMAGE_REF:-postgres:16-alpine}
project="glyphflow-deployment-check-$$"
network="$project-network"
partial_nats="$project-nats"
partial_postgres="$project-postgres"
partial_controlplane="$project-controlplane"
partial_port=${PARTIAL_CONTROLPLANE_PORT:-18081}
production_config=$(mktemp)

cleanup() {
  docker compose -p "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker rm -f "$partial_controlplane" "$partial_postgres" "$partial_nats" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -f "$production_config"
}
trap cleanup EXIT

command -v docker >/dev/null || { echo "deployment check: Docker is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "deployment check: curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "deployment check: jq is required" >&2; exit 1; }

wait_for_url() {
  local url=$1
  local attempt=0
  while [ "$attempt" -lt 60 ]; do
    if curl --fail --silent "$url" >/dev/null 2>&1; then
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 1
  done
  echo "deployment check: timed out waiting for $url" >&2
  return 1
}

export COMPOSE_PROJECT_NAME="$project"
export GLYPHFLOW_IMAGE="$image"
export GLYPHFLOW_PORT="${GLYPHFLOW_PORT:-18080}"
export POSTGRES_HOST_PORT="${POSTGRES_HOST_PORT:-127.0.0.1:15432}"
export NATS_HOST_PORT="${NATS_HOST_PORT:-127.0.0.1:14222}"
export NATS_MONITOR_HOST_PORT="${NATS_MONITOR_HOST_PORT:-127.0.0.1:18222}"

export DATABASE_URL_FILE=/dev/null
export NATS_URL_FILE=/dev/null
export ACCESS_TOKEN_SECRET_FILE=/dev/null
export CONTROL_PLANE_SIGNING_PRIVATE_KEY_FILE=/dev/null
export PASSWORD_PEPPER_FILE=/dev/null
export SECRET_ENCRYPTION_KEY_FILE=/dev/null
export GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE=/dev/null
export POSTGRES_PASSWORD_FILE=/dev/null
export POSTGRES_CERT_SOURCE=/dev/null
export POSTGRES_KEY_SOURCE=/dev/null
export POSTGRES_CA_SOURCE=/dev/null
export NATS_CERT_SOURCE=/dev/null
export NATS_KEY_SOURCE=/dev/null
export NATS_CA_SOURCE=/dev/null
export WEB_ORIGIN=https://console.example
export CORS_ORIGIN=https://console.example
export CSRF_ORIGINS=https://console.example
export GLYPHFLOW_BOOTSTRAP_EMAIL=admin@example.com
export GLYPHFLOW_SYSTEM_ADMINS=admin@example.com
export GLYPHFLOW_NETWORK="$network"
export RUNNER_NATS_URL=tls://nats:4222
export RUNNER_CONTROL_PLANE_URL=https://controlplane.example

docker compose -f compose.yaml -f compose.production.yaml config --format json > "$production_config"
jq -e '
  .networks.default.internal == true and
  (.services.postgres.ports // []) == [] and
  (.services.nats.ports // []) == [] and
  .services.controlplane.user == "65532:65532" and
  (.services.controlplane.secrets | length) >= 11
' "$production_config" >/dev/null

docker compose -p "$project" -f compose.yaml up -d --wait
wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/healthz"
wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/readyz"
docker compose -p "$project" restart controlplane >/dev/null
wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/readyz"

docker network create "$network" >/dev/null
docker run -d --name "$partial_nats" --network "$network" --network-alias nats "$nats_image" --jetstream --http_port 8222 >/dev/null
docker run -d --name "$partial_postgres" --network "$network" --network-alias postgres \
  -e POSTGRES_DB=glyphflow -e POSTGRES_USER=glyphflow -e POSTGRES_PASSWORD=glyphflow "$postgres_image" >/dev/null
attempt=0
while ! docker exec "$partial_postgres" pg_isready -U glyphflow -d glyphflow >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 60 ] || { echo "deployment check: PostgreSQL did not become ready" >&2; exit 1; }
  sleep 1
done
attempt=0
while ! docker exec "$partial_nats" wget -q -O - http://127.0.0.1:8222/healthz >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  [ "$attempt" -lt 60 ] || { echo "deployment check: NATS did not become ready" >&2; exit 1; }
  sleep 1
done
docker run -d --name "$partial_controlplane" --network "$network" -p "${partial_port}:8080" \
  --tmpfs /data \
  -e DATABASE_URL='postgres://glyphflow:glyphflow@postgres:5432/glyphflow?sslmode=disable' \
  -e NATS_URL='nats://nats:4222' \
  -e ACCESS_TOKEN_SECRET='deployment-check-secret-012345678901234567' \
  -e PASSWORD_PEPPER='deployment-check-pepper' \
  -e WEB_ORIGIN="http://127.0.0.1:${partial_port}" \
  -e CORS_ORIGIN="http://127.0.0.1:${partial_port}" \
  -e CSRF_ORIGINS="http://127.0.0.1:${partial_port}" \
  -e ENVIRONMENT=development -e ALLOW_INSECURE_TRANSPORT=true \
  -e DATA_DIR=/data -e MAX_MESSAGE_BYTES=1048576 \
  -e GLYPHFLOW_BOOTSTRAP_EMAIL=admin@example.com -e GLYPHFLOW_SYSTEM_ADMINS=admin@example.com \
  -e ENABLE_PASSWORD_LOGIN=false -e ENABLE_PASSWORD_REGISTRATION=false \
  -e RUNNER_NATS_URL=nats://nats:4222 -e RUNNER_CONTROL_PLANE_URL="http://127.0.0.1:${partial_port}" \
  --entrypoint /usr/local/bin/glyphflow-controlplane "$image" >/dev/null
wait_for_url "http://127.0.0.1:${partial_port}/api/v1/healthz"
wait_for_url "http://127.0.0.1:${partial_port}/api/v1/readyz"
docker restart "$partial_controlplane" >/dev/null
wait_for_url "http://127.0.0.1:${partial_port}/api/v1/readyz"
docker inspect --format '{{json .NetworkSettings.Networks}}' "$partial_controlplane" | jq -e --arg network "$network" 'has($network)' >/dev/null
echo "deployment topology checks: PASS"
