#!/usr/bin/env bash
set -Eeuo pipefail

project_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
backend_pid=''
frontend_pid=''

cleanup() {
  trap - EXIT INT TERM
  [[ -n "$frontend_pid" ]] && kill "$frontend_pid" 2>/dev/null || true
  [[ -n "$backend_pid" ]] && kill "$backend_pid" 2>/dev/null || true
  wait 2>/dev/null || true
  docker compose down >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cd "$project_root"
docker compose up -d postgres nats

if [[ ! -d "$project_root/frontend/node_modules" ]]; then
  (cd "$project_root/frontend" && npm ci)
fi

mkdir -p "${DATA_DIR:-$project_root/.dev-data}"

(
  cd "$project_root/backend"
  DATABASE_URL="${DATABASE_URL:-postgres://glyphflow:glyphflow@localhost:5432/glyphflow?sslmode=disable}" \
  NATS_URL="${NATS_URL:-nats://localhost:4222}" \
  ACCESS_TOKEN_SECRET="${ACCESS_TOKEN_SECRET:-development-secret-at-least-32-characters}" \
  PASSWORD_PEPPER="${PASSWORD_PEPPER:-development-password-pepper-at-least-16}" \
  WEB_ORIGIN="${WEB_ORIGIN:-http://${FRONTEND_HOST:-127.0.0.1}:5173}" \
  DATA_DIR="${DATA_DIR:-$project_root/.dev-data}" \
  MAX_MESSAGE_BYTES="${MAX_MESSAGE_BYTES:-1048576}" \
  GLYPHFLOW_BOOTSTRAP_USERNAME="${GLYPHFLOW_BOOTSTRAP_USERNAME:-admin}" \
  GLYPHFLOW_BOOTSTRAP_PASSWORD="${GLYPHFLOW_BOOTSTRAP_PASSWORD:-admin-password-123}" \
  go run ./cmd/controlplane
) &
backend_pid=$!

(
  cd "$project_root/frontend"
  VITE_API_URL="${VITE_API_URL:-}" \
  VITE_BACKEND_URL="${VITE_BACKEND_URL:-http://localhost:8080}" \
  npm run dev -- --host "${FRONTEND_HOST:-127.0.0.1}"
) &
frontend_pid=$!

echo "Frontend: http://${FRONTEND_HOST:-127.0.0.1}:5173"
echo "Backend:  http://localhost:8080"
echo "Press Ctrl-C to stop both processes and Docker dependencies."

wait -n "$backend_pid" "$frontend_pid"
