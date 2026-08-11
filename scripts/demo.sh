#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export TIME_SCALE="${TIME_SCALE:-86400}"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-afterwords-demo-$$}"
echo "Starting Afterwords demo: one real second equals ${TIME_SCALE} virtual seconds."
echo "Dashboard: http://localhost:5173"
echo "Mailpit:   http://localhost:8025"
echo "Ephemeral Compose project: ${COMPOSE_PROJECT_NAME}"

cleanup() {
  if [[ "${KEEP_DEMO_RUNNING:-0}" != "1" ]]; then
    docker compose -f docker-compose.dev.yml down -v --remove-orphans
  fi
}
trap cleanup EXIT

docker compose -f docker-compose.dev.yml up --build -d

ready=0
for ((attempt = 1; attempt <= 90; attempt++)); do
  if curl --fail --silent http://localhost:8080/healthz >/dev/null \
    && curl --fail --silent http://localhost:5173/ >/dev/null \
    && curl --fail --silent http://localhost:8025/api/v1/messages >/dev/null; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" != "1" ]]; then
  echo "The API, Vite, or Mailpit did not become ready within 90 seconds." >&2
  docker compose -f docker-compose.dev.yml logs --tail=80 >&2
  exit 1
fi

if [[ ! -f e2e/package.json ]]; then
  echo "The Playwright demo package is missing; refusing to report a no-op as a successful demo." >&2
  exit 1
fi
npm ci --prefix e2e
export AFTERWORDS_E2E_BASE_URL="${AFTERWORDS_E2E_BASE_URL:-http://localhost:5173}"
export MAILPIT_URL="${MAILPIT_URL:-http://localhost:8025}"
npm run demo --prefix e2e

echo "Demo completed. Set KEEP_DEMO_RUNNING=1 to leave the stack running."
