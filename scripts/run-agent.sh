#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

mkdir -p .infraos/logs .infraos/pids

API_URL="${INFRAOS_AGENT_API_URL:-http://127.0.0.1:8080}"
CREDS_PATH="${INFRAOS_AGENT_CREDENTIALS_PATH:-./.infraos/agent-credentials.json}"
INTERVAL="${INFRAOS_AGENT_INTERVAL_SECONDS:-10}"

if [[ ! -x ./bin/infraos-agent ]]; then
  GO_BIN="$(command -v go 2>/dev/null || echo "$HOME/.local/go/bin/go")"
  "$GO_BIN" build -o bin/infraos-agent ./cmd/agent
fi

if [[ ! -f "$CREDS_PATH" && -z "${INFRAOS_AGENT_ENROLL_TOKEN:-}" ]]; then
  echo "Provide INFRAOS_AGENT_ENROLL_TOKEN from Workspace -> Hosts -> Enroll Agent to enroll this machine." >&2
  exit 1
fi

./bin/infraos-agent \
  --api-url "$API_URL" \
  --credentials-file "$CREDS_PATH" \
  --interval "$INTERVAL" \
  > .infraos/logs/agent.log 2>&1 &
echo $! > .infraos/pids/agent.pid
echo "InfraOS local agent started (PID $(cat .infraos/pids/agent.pid)), logging to .infraos/logs/agent.log"
