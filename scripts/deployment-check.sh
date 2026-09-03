#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
image=${1:?usage: $0 APPLICATION_IMAGE}
nats_image=${NATS_IMAGE_REF:-nats@sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927}
postgres_image=${POSTGRES_IMAGE_REF:-postgres@sha256:44c4ee9810eff91f7eab4d822642e01115b1a9eccce4bcbdde7604752d68eac6}
project="glyphflow-deployment-check-$$"
network="$project-network"
partial_nats="$project-nats"
partial_postgres="$project-postgres"
partial_controlplane="$project-controlplane"
partial_port=${PARTIAL_CONTROLPLANE_PORT:-18081}
partial_nats_port=${PARTIAL_NATS_PORT:-14223}
production_project="$project-production"
production_port=${PRODUCTION_CONTROLPLANE_PORT:-18082}
test_tmp=$(mktemp -d)
production_dir="$test_tmp/production"
production_config="$test_tmp/production-config.json"
worker_pid=""
deployment_check_postgres_password=${DEPLOYMENT_CHECK_POSTGRES_PASSWORD:-$(openssl rand -hex 16)}

cleanup() {
  if [ -n "$worker_pid" ]; then
    kill "$worker_pid" >/dev/null 2>&1 || true
    wait "$worker_pid" >/dev/null 2>&1 || true
  fi
  COMPOSE_PROJECT_NAME="$production_project" docker compose -p "$production_project" -f compose.yaml -f compose.production.yaml down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker compose -p "$project" down --volumes --remove-orphans >/dev/null 2>&1 || true
  docker rm -f "$partial_controlplane" "$partial_postgres" "$partial_nats" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$test_tmp"
}
trap cleanup EXIT

command -v docker >/dev/null || { echo "deployment check: Docker is required" >&2; exit 1; }
command -v curl >/dev/null || { echo "deployment check: curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "deployment check: jq is required" >&2; exit 1; }
command -v openssl >/dev/null || { echo "deployment check: openssl is required" >&2; exit 1; }
command -v setsid >/dev/null || { echo "deployment check: setsid is required" >&2; exit 1; }
command -v base64 >/dev/null || { echo "deployment check: base64 is required" >&2; exit 1; }

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

make_certificate() {
  local name=$1
  local usage=$2
  printf 'subjectAltName=DNS:%s\nextendedKeyUsage=%s\n' "$name" "$usage" > "$production_dir/$name.ext"
  openssl req -newkey rsa:2048 -nodes -subj "/CN=$name" \
    -keyout "$production_dir/$name.key" -out "$production_dir/$name.csr" >/dev/null 2>&1
  openssl x509 -req -days 1 -sha256 -in "$production_dir/$name.csr" \
    -CA "$production_dir/ca.crt" -CAkey "$production_dir/ca.key" \
    -CAcreateserial -out "$production_dir/$name.crt" \
    -extfile "$production_dir/$name.ext" >/dev/null 2>&1
}

create_production_fixtures() {
  mkdir -p "$production_dir"
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=Glyphflow deployment check CA' \
    -keyout "$production_dir/ca.key" -out "$production_dir/ca.crt" >/dev/null 2>&1
  make_certificate nats 'serverAuth,clientAuth'
  make_certificate postgres 'serverAuth'

  printf '%s\n' "$deployment_check_postgres_password" > "$production_dir/postgres-password"
  printf '%s\n' "postgres://glyphflow:${deployment_check_postgres_password}@postgres:5432/glyphflow?sslmode=verify-full" > "$production_dir/database-url"
  printf '%s\n' 'tls://nats:4222' > "$production_dir/nats-url"
  printf '%s\n' 'deployment-check-access-token-secret-0123456789' > "$production_dir/access-token-secret"
  head -c 64 /dev/urandom | base64 -w0 | tr -d '=' > "$production_dir/control-plane-signing-key"
  printf '\n' >> "$production_dir/control-plane-signing-key"
  printf '%s\n' 'deployment-check-password-pepper' > "$production_dir/password-pepper"
  printf '%s\n' 'deployment-check-bootstrap-password' > "$production_dir/bootstrap-password"

  docker run --rm -v "$production_dir:/fixtures" "$nats_image" sh -ec '
    chown 65532:65532 /fixtures/database-url /fixtures/nats-url /fixtures/access-token-secret /fixtures/control-plane-signing-key /fixtures/password-pepper /fixtures/bootstrap-password
    chown 70:70 /fixtures/postgres-password /fixtures/postgres.key
    chmod 0400 /fixtures/database-url /fixtures/nats-url /fixtures/access-token-secret /fixtures/control-plane-signing-key /fixtures/password-pepper /fixtures/bootstrap-password /fixtures/postgres-password /fixtures/postgres.key
    chmod 0444 /fixtures/ca.crt /fixtures/nats.crt /fixtures/nats.key /fixtures/postgres.crt
  '
  docker volume create "${production_project}_nats-data" >/dev/null
  docker run --rm -v "${production_project}_nats-data:/data" "$nats_image" sh -ec 'chown -R 1000:1000 /data'

  export DATABASE_URL_FILE="$production_dir/database-url"
  export NATS_URL_FILE="$production_dir/nats-url"
  export ACCESS_TOKEN_SECRET_FILE="$production_dir/access-token-secret"
  export CONTROL_PLANE_SIGNING_PRIVATE_KEY_FILE="$production_dir/control-plane-signing-key"
  export PASSWORD_PEPPER_FILE="$production_dir/password-pepper"
  export GLYPHFLOW_BOOTSTRAP_PASSWORD_FILE="$production_dir/bootstrap-password"
  export POSTGRES_PASSWORD_FILE="$production_dir/postgres-password"
  export POSTGRES_CERT_SOURCE="$production_dir/postgres.crt"
  export POSTGRES_KEY_SOURCE="$production_dir/postgres.key"
  export POSTGRES_CA_SOURCE="$production_dir/ca.crt"
  export NATS_CERT_SOURCE="$production_dir/nats.crt"
  export NATS_KEY_SOURCE="$production_dir/nats.key"
  export NATS_CA_SOURCE="$production_dir/ca.crt"
}

run_production_smoke() {
  COMPOSE_PROJECT_NAME="$production_project" GLYPHFLOW_IMAGE="$image" GLYPHFLOW_PORT="$production_port" \
    NATS_IMAGE="$nats_image" POSTGRES_IMAGE="$postgres_image" WEB_ORIGIN=https://console.example \
    CORS_ORIGIN=https://console.example CSRF_ORIGINS=https://console.example \
    GLYPHFLOW_BOOTSTRAP_EMAIL=admin@example.com GLYPHFLOW_SYSTEM_ADMINS=admin@example.com \
    docker compose -p "$production_project" -f compose.yaml -f compose.production.yaml up -d --wait
}

run_dispatch_check() {
  local base_url=$1
  local nats_endpoint=$2
  local origin=$3
  local label=$4
  local dispatch_dir="$test_tmp/dispatch-$label"
  local cookie_jar="$dispatch_dir/cookies"
  local csrf login_payload enrollment_payload enrollment_json artifact runner_id runner_path
  local task_json task_id run_json run_id run_state runner_json
  mkdir -p "$dispatch_dir/worker-data"

  curl --fail --silent --show-error -c "$cookie_jar" "$base_url/api/v1/config" >/dev/null
  csrf=$(awk '$6 == "glyphflow_csrf" {print $7}' "$cookie_jar")
  [ -n "$csrf" ] || { echo "deployment check: CSRF token was not issued" >&2; return 1; }
  login_payload=$(jq -nc --arg email admin@example.com --arg password "$DEPLOYMENT_CHECK_PASSWORD" '{email:$email,password:$password}')
  curl --fail --silent --show-error -b "$cookie_jar" -c "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $origin" -H "X-CSRF-Token: $csrf" \
    --data "$login_payload" "$base_url/api/v1/auth/login" >/dev/null

  enrollment_payload=$(jq -nc --arg name "deployment-check-$label-$$" --arg control "$base_url" \
    --arg nats "$nats_endpoint" '{runner_name:$name,pool_id:"default",platform:"linux",architecture:"amd64",capacity:1,ui:"headless",control_plane_url:$control,embedded_nats_endpoint:$nats}')
  enrollment_json="$dispatch_dir/enrollment.json"
  curl --fail --silent --show-error -b "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $origin" -H "X-CSRF-Token: $csrf" \
    --data "$enrollment_payload" "$base_url/api/v1/runners/enrollments" > "$enrollment_json"
  artifact=$(jq -r '.artifact // empty' "$enrollment_json")
  runner_id=$(jq -r '.runner_id // empty' "$enrollment_json")
  [ -n "$artifact" ] && [ -n "$runner_id" ] || { echo "deployment check: runner artifact was not returned" >&2; return 1; }
  runner_path="$dispatch_dir/runner"
  printf '%s' "$artifact" | base64 -d > "$runner_path"
  chmod 700 "$runner_path"

  setsid env GLYPHFLOW_NATS_ENDPOINT="$nats_endpoint" GLYPHFLOW_CONTROL_PLANE_URL="$base_url" \
      DATA_DIR="$dispatch_dir/worker-data" ENVIRONMENT=development ALLOW_INSECURE_TRANSPORT=true \
      "$runner_path" > "$dispatch_dir/worker.log" 2>&1 < /dev/null &
  worker_pid=$!

  local attempt=0
  while :; do
    kill -0 "$worker_pid" >/dev/null 2>&1 || { cat "$dispatch_dir/worker.log" >&2; return 1; }
    runner_json=$(curl --fail --silent --show-error -b "$cookie_jar" "$base_url/api/v1/runners")
    if printf '%s' "$runner_json" | jq -e --arg id "$runner_id" '.items[]? | select(.id == $id and .observedState == "ONLINE")' >/dev/null; then
      break
    fi
    attempt=$((attempt + 1))
    [ "$attempt" -lt 60 ] || { echo "deployment check: worker did not become online" >&2; return 1; }
    sleep 1
  done

  task_json="$dispatch_dir/task.json"
  curl --fail --silent --show-error -b "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $origin" -H "X-CSRF-Token: $csrf" \
    --data '{"name":"deployment-check","command":["printf","deployment-check"],"runner_pool":"default","duration_seconds":30,"max_output_bytes":1024}' \
    "$base_url/api/v1/tasks" > "$task_json"
  task_id=$(jq -r '.id // empty' "$task_json")
  [ -n "$task_id" ] || { echo "deployment check: task was not created" >&2; return 1; }
  run_json="$dispatch_dir/run.json"
  curl --fail --silent --show-error -b "$cookie_jar" \
    -H 'Content-Type: application/json' -H "Origin: $origin" -H "X-CSRF-Token: $csrf" \
    --data "$(jq -nc --arg id "$task_id" '{task_id:$id}')" "$base_url/api/v1/runs/execute" > "$run_json"
  run_id=$(jq -r '.id // empty' "$run_json")
  [ -n "$run_id" ] || { echo "deployment check: run was not created" >&2; return 1; }

  attempt=0
  while :; do
    run_json=$(curl --fail --silent --show-error -b "$cookie_jar" "$base_url/api/v1/runs/$run_id")
    run_state=$(printf '%s' "$run_json" | jq -r '.state')
    case "$run_state" in
      SUCCEEDED) break ;;
      FAILED|TIMED_OUT|UNKNOWN|CANCELLED)
        printf '%s\n' "$run_json" >&2
        return 1
        ;;
      *)
        printf 'deployment check: unexpected run state: %s\n' "$run_state" >&2
        return 1
        ;;
    esac
    attempt=$((attempt + 1))
    [ "$attempt" -lt 60 ] || { echo "deployment check: run did not finish" >&2; return 1; }
    sleep 1
  done
  printf '%s' "$run_json" | jq -e --arg runner "$runner_id" '.runner == $runner and .exitCode == 0' >/dev/null
  curl --fail --silent --show-error -b "$cookie_jar" \
    "$base_url/api/v1/runs/$run_id/logs?stream=stdout" | jq -s -e 'map(select(.text == "deployment-check")) | length == 1' >/dev/null
  kill "$worker_pid" >/dev/null 2>&1 || true
  wait "$worker_pid" >/dev/null 2>&1 || true
  worker_pid=""
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
  .services.controlplane.environment.PGSSLROOTCERT == "/run/secrets/postgres-ca" and
  (.services.controlplane.secrets | length) >= 11
' "$production_config" >/dev/null

create_production_fixtures
run_production_smoke

export DEPLOYMENT_CHECK_PASSWORD=deployment-check-bootstrap-password
export GLYPHFLOW_BOOTSTRAP_PASSWORD="$DEPLOYMENT_CHECK_PASSWORD"
export WEB_ORIGIN=http://localhost
export CORS_ORIGIN=http://localhost
export CSRF_ORIGINS=http://localhost
export RUNNER_NATS_URL=nats://nats:4222
export RUNNER_CONTROL_PLANE_URL="http://127.0.0.1:${GLYPHFLOW_PORT}"

COMPOSE_PROJECT_NAME="${project}-partial" GLYPHFLOW_IMAGE="$image" GLYPHFLOW_NETWORK="$network" \
  WEB_ORIGIN=https://console.example CORS_ORIGIN=https://console.example CSRF_ORIGINS=https://console.example \
  RUNNER_NATS_URL=tls://nats:4222 RUNNER_CONTROL_PLANE_URL=https://controlplane.example \
  docker compose -f compose.partial.yaml config --format json > "$test_tmp/partial-config.json"
jq -e '
  .networks.glyphflow.external == true and
  .services.controlplane.environment.GLYPHFLOW_DISABLE_NGINX == "true" and
  .services.controlplane.environment.PGSSLROOTCERT == "/run/secrets/postgres-ca" and
  (.services.controlplane.secrets | length) >= 11
' "$test_tmp/partial-config.json" >/dev/null

docker compose -p "$project" -f compose.yaml up -d --wait
wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/healthz"
wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/readyz"
run_dispatch_check "http://127.0.0.1:${GLYPHFLOW_PORT}" "nats://127.0.0.1:${NATS_HOST_PORT##*:}" http://localhost full
for service in nats postgres controlplane web; do
  docker compose -p "$project" restart "$service" >/dev/null
  wait_for_url "http://127.0.0.1:${GLYPHFLOW_PORT}/api/v1/readyz"
done

docker network create "$network" >/dev/null
docker run -d --name "$partial_nats" --network "$network" --network-alias nats \
  -p "127.0.0.1:${partial_nats_port}:4222" "$nats_image" --jetstream --http_port 8222 >/dev/null
docker run -d --name "$partial_postgres" --network "$network" --network-alias postgres \
  -e POSTGRES_DB=glyphflow -e POSTGRES_USER=glyphflow -e POSTGRES_PASSWORD="$deployment_check_postgres_password" "$postgres_image" >/dev/null
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
  -e DATABASE_URL="postgres://glyphflow:${deployment_check_postgres_password}@postgres:5432/glyphflow?sslmode=disable" \
  -e NATS_URL='nats://nats:4222' \
  -e ACCESS_TOKEN_SECRET='deployment-check-secret-012345678901234567' \
  -e PASSWORD_PEPPER='deployment-check-pepper' \
  -e WEB_ORIGIN="http://127.0.0.1:${partial_port}" \
  -e CORS_ORIGIN="http://127.0.0.1:${partial_port}" \
  -e CSRF_ORIGINS="http://127.0.0.1:${partial_port}" \
  -e ENVIRONMENT=development -e ALLOW_INSECURE_TRANSPORT=true \
  -e DATA_DIR=/data -e MAX_MESSAGE_BYTES=1048576 \
  -e GLYPHFLOW_BOOTSTRAP_EMAIL=admin@example.com -e GLYPHFLOW_SYSTEM_ADMINS=admin@example.com \
  -e GLYPHFLOW_BOOTSTRAP_PASSWORD="$DEPLOYMENT_CHECK_PASSWORD" \
  -e ENABLE_PASSWORD_LOGIN=true -e ENABLE_PASSWORD_REGISTRATION=false \
  -e RUNNER_NATS_URL=nats://nats:4222 -e RUNNER_CONTROL_PLANE_URL="http://127.0.0.1:${partial_port}" \
  -e RUNNER_BINARIES_DIR=/app/runner-binaries \
  --entrypoint /usr/local/bin/glyphflow-controlplane "$image" >/dev/null
wait_for_url "http://127.0.0.1:${partial_port}/api/v1/healthz"
wait_for_url "http://127.0.0.1:${partial_port}/api/v1/readyz"
run_dispatch_check "http://127.0.0.1:${partial_port}" "nats://127.0.0.1:${partial_nats_port}" "http://127.0.0.1:${partial_port}" partial
for container in "$partial_nats" "$partial_postgres" "$partial_controlplane"; do
  docker restart "$container" >/dev/null
  wait_for_url "http://127.0.0.1:${partial_port}/api/v1/readyz"
done
for container in "$partial_nats" "$partial_postgres" "$partial_controlplane"; do
  docker inspect --format '{{json .NetworkSettings.Networks}}' "$container" | jq -e --arg network "$network" 'has($network)' >/dev/null
done
echo "deployment topology checks: PASS"
