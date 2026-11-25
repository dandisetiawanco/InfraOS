# Troubleshooting

## Port 3000 or 8080 is already in use
`scripts/dev.sh` checks ports `3000` and `8080` before launching. If another local application already listens on `3000`, `scripts/dev.sh` automatically selects `3001` for the InfraOS web app without terminating the existing service. You can override ports explicitly with `INFRAOS_WEB_PORT` and `INFRAOS_API_PORT`.

## Agent fails to connect or enroll
- Enrollment tokens expire after 30 minutes and are single-use. Generate a fresh token in **Workspace -> Hosts -> Enroll host agent**.
- Confirm `INFRAOS_AGENT_API_URL` matches the running API port (default `http://127.0.0.1:8080`).
- Check `.infraos/logs/agent.log` for structured JSON diagnostics.

## Service check rejected as blocked destination
InfraOS blocks private RFC1918 networks, link-local ranges, cloud metadata IPs, and loopback addresses by default. For local development endpoints, explicitly list the exact origin in `INFRAOS_MONITORING_LOCAL_ALLOWLIST` (e.g., `http://127.0.0.1:8080,http://127.0.0.1:8090`).

## Docker containers page shows Disabled or Unavailable
Docker integration is disabled by default so the lightweight profile runs without Docker Desktop. On an authorized host with Docker running, set `INFRAOS_DOCKER_ENABLED=true` and verify `/var/run/docker.sock` permissions.

## Stream indicator shows Polling instead of SSE live
If an intermediate proxy buffers Server-Sent Events or the API restarted, the browser automatically falls back to 15-second TanStack Query polling and reconnects SSE with exponential backoff. In Nginx deployments, ensure `proxy_buffering off` is configured on `/api/v1/events/stream`.
