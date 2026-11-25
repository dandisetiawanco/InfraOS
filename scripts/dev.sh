#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p .infraos/logs .infraos/pids data backups

GO_BIN="$(command -v go 2>/dev/null || echo "$HOME/.local/go/bin/go")"
if ! command -v "$GO_BIN" >/dev/null 2>&1; then
  echo "Error: Go is required to run InfraOS." >&2
  exit 1
fi
if ! command -v node >/dev/null 2>&1 || ! command -v pnpm >/dev/null 2>&1; then
  echo "Error: Node.js and pnpm are required to run InfraOS." >&2
  exit 1
fi

is_port_busy() {
  lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
}

API_PORT="${INFRAOS_API_PORT:-8080}"
if is_port_busy "$API_PORT"; then
  echo "Port $API_PORT is occupied by another process; selecting fallback API port 8081."
  API_PORT=8081
fi

WEB_PORT="${INFRAOS_WEB_PORT:-3000}"
if is_port_busy "$WEB_PORT"; then
  echo "Port $WEB_PORT is occupied by another process; selecting fallback frontend port 3001."
  WEB_PORT=3001
fi

export INFRAOS_PROFILE="${INFRAOS_PROFILE:-lightweight}"
export INFRAOS_API_HOST="${INFRAOS_API_HOST:-127.0.0.1}"
export INFRAOS_API_PORT="$API_PORT"
export INFRAOS_WEB_URL="http://localhost:${WEB_PORT}"
export INFRAOS_ALLOWED_ORIGINS="http://localhost:${WEB_PORT},http://127.0.0.1:${WEB_PORT},http://localhost:3000,http://localhost:3001"
export INFRAOS_MONITORING_LOCAL_ALLOWLIST="http://127.0.0.1:${API_PORT},http://localhost:${API_PORT},http://127.0.0.1:8090,http://localhost:8090"
export NEXT_PUBLIC_API_URL="http://127.0.0.1:${API_PORT}"

echo "Building InfraOS Go binaries..."
"$GO_BIN" build -o bin/infraos-api ./cmd/api
"$GO_BIN" build -o bin/infraos-agent ./cmd/agent

echo "Starting InfraOS API on http://127.0.0.1:${API_PORT}..."
./bin/infraos-api > .infraos/logs/api.log 2>&1 &
API_PID=$!
echo "$API_PID" > .infraos/pids/api.pid

for _ in $(seq 1 30); do
  if curl -fsS "http://127.0.0.1:${API_PORT}/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

echo "Starting InfraOS web frontend on http://localhost:${WEB_PORT}..."
PORT="$WEB_PORT" NEXT_PUBLIC_API_URL="http://127.0.0.1:${API_PORT}" pnpm --filter @infraos/web dev --port "$WEB_PORT" > .infraos/logs/web.log 2>&1 &
WEB_PID=$!
echo "$WEB_PID" > .infraos/pids/web.pid

if [[ "${INFRAOS_START_AGENT:-false}" == "true" ]]; then
  echo "Starting local InfraOS agent..."
  ./scripts/run-agent.sh &
fi

echo ""
echo "InfraOS lightweight profile started:"
echo "  Frontend:  http://localhost:${WEB_PORT}"
echo "  API:       http://127.0.0.1:${API_PORT}"
echo "  API Docs:  http://127.0.0.1:${API_PORT}/api/docs"
echo "  Metrics:   http://127.0.0.1:${API_PORT}/metrics"
echo "  Logs:      .infraos/logs/"
echo "  Stop:      ./scripts/stop.sh"
