#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_pid=''
frontend_pid=''
data_dir="${DATA_DIR:-$project_root/.dev-data}"

cleanup() {
  trap - EXIT INT TERM
  [[ -n "$frontend_pid" ]] && kill "$frontend_pid" 2>/dev/null || true
  [[ -n "$backend_pid" ]] && kill "$backend_pid" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cd "$project_root"
version="$(tr -d '[:space:]' < "$project_root/VERSION")"

if [[ ! -d "$project_root/frontend/node_modules" ]]; then
  (cd "$project_root/frontend" && npm ci --ignore-scripts)
fi

mkdir -p "$data_dir"
bash "$project_root/backend/build_runner_binaries.sh"

(
  cd "$project_root/backend"
  GLYPHFLOW_DATABASE="${GLYPHFLOW_DATABASE:-sqlite}" \
  GLYPHFLOW_NATS="${GLYPHFLOW_NATS:-embed}" \
  DATABASE_URL="${DATABASE_URL:-}" \
  NATS_URL="${NATS_URL:-}" \
  ACCESS_TOKEN_SECRET="${ACCESS_TOKEN_SECRET:-development-secret-at-least-32-characters}" \
  PASSWORD_PEPPER="${PASSWORD_PEPPER:-development-password-pepper-at-least-16}" \
  WEB_ORIGIN="${WEB_ORIGIN:-http://${FRONTEND_HOST:-0.0.0.0}:5173}" \
  CSRF_ORIGINS="${CSRF_ORIGINS:-http://localhost:5173,http://127.0.0.1:5173,http://${FRONTEND_HOST:-0.0.0.0}:5173}" \
  CORS_ORIGIN="${CORS_ORIGIN:-*}" \
  RUNNER_NATS_URL="${RUNNER_NATS_URL:-}" \
  RUNNER_CONTROL_PLANE_URL="${RUNNER_CONTROL_PLANE_URL:-http://localhost:8080}" \
  DATABASE_STORAGE_CAPACITY_BYTES="${DATABASE_STORAGE_CAPACITY_BYTES:-1073741824}" \
  ENVIRONMENT="${ENVIRONMENT:-development}" \
  ALLOW_INSECURE_TRANSPORT="${ALLOW_INSECURE_TRANSPORT:-true}" \
  DATA_DIR="$data_dir" \
  MAX_MESSAGE_BYTES="${MAX_MESSAGE_BYTES:-1048576}" \
  GLYPHFLOW_BOOTSTRAP_EMAIL="${GLYPHFLOW_BOOTSTRAP_EMAIL:-admin@domain.com}" \
  GLYPHFLOW_BOOTSTRAP_PASSWORD="${GLYPHFLOW_BOOTSTRAP_PASSWORD:-password}" \
  GLYPHFLOW_SYSTEM_ADMINS="${GLYPHFLOW_SYSTEM_ADMINS:-admin@domain.com}" \
  ENABLE_PASSWORD_LOGIN="${ENABLE_PASSWORD_LOGIN:-true}" \
  ENABLE_PASSWORD_REGISTRATION="${ENABLE_PASSWORD_REGISTRATION:-true}" \
  DEFAULT_ROLE_ID="${DEFAULT_ROLE_ID:-system-user}" \
  RUNNER_BINARIES_DIR="${RUNNER_BINARIES_DIR:-$project_root/backend/runner-binaries}" \
  go run -ldflags="-X github.com/VBenevides/Glyphflow/backend.Version=$version" ./cmd/controlplane
) &
backend_pid=$!

(
  cd "$project_root/frontend"
  # Local development only; production endpoints must use HTTPS.
  VITE_BACKEND_URL="${VITE_BACKEND_URL:-http://localhost:8080}" \
  npm run dev -- --host "${FRONTEND_HOST:-0.0.0.0}"
) &
frontend_pid=$!

# Local development only; production endpoints must use HTTPS.
echo "Frontend: http://${FRONTEND_HOST:-0.0.0.0}:5173"
echo "Backend:  http://0.0.0.0:8080"
echo "Press Ctrl-C to stop both processes."

wait -n "$backend_pid" "$frontend_pid"
