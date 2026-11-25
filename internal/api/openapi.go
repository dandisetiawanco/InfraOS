package api

import "net/http"

func (s *Server) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	spec := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "InfraOS Control Plane API",
			"version":     "1.0.0",
			"description": "Versioned REST API for infrastructure monitoring, agent enrollment, service checks, controlled deployments, centralized logs, alerts, incidents, integrations, and audit trails.",
		},
		"servers": []map[string]string{
			{"url": "/api/v1", "description": "Current API server"},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"sessionCookie": map[string]string{"type": "apiKey", "in": "cookie", "name": "infraos_session"},
				"agentBearer":   map[string]string{"type": "apiKey", "in": "header", "name": "Authorization"},
			},
		},
		"paths": map[string]any{
			"/auth/register":                map[string]any{"post": map[string]any{"summary": "Register user and default workspace organization"}},
			"/auth/login":                   map[string]any{"post": map[string]any{"summary": "Authenticate user with Argon2id verification and rate limiting"}},
			"/auth/logout":                  map[string]any{"post": map[string]any{"summary": "Revoke active session"}},
			"/auth/demo":                    map[string]any{"post": map[string]any{"summary": "Launch isolated synthetic demonstration session"}},
			"/auth/me":                      map[string]any{"get": map[string]any{"summary": "Get active session user, organization, role, and organizations list"}},
			"/overview":                     map[string]any{"get": map[string]any{"summary": "Aggregated infrastructure overview with metric freshness metadata"}},
			"/events/stream":                map[string]any{"get": map[string]any{"summary": "Server-Sent Events stream for live host metrics, checks, deployments, logs, and alerts"}},
			"/agents/enrollment-tokens":     map[string]any{"post": map[string]any{"summary": "Issue expiring one-time enrollment token (admin/owner)"}},
			"/agents/enroll":                map[string]any{"post": map[string]any{"summary": "Exchange one-time enrollment token for per-agent credential"}},
			"/agents/metrics":               map[string]any{"post": map[string]any{"summary": "Ingest real host telemetry authenticated via Authorization: Agent <token>"}},
			"/agents/{id}/rotate":           map[string]any{"post": map[string]any{"summary": "Rotate active agent credential"}},
			"/agents/{id}/revoke":           map[string]any{"post": map[string]any{"summary": "Revoke agent and invalidate its credentials"}},
			"/hosts":                        map[string]any{"get": map[string]any{"summary": "List connected hosts with search and environment filtering"}},
			"/hosts/{id}":                   map[string]any{"get": map[string]any{"summary": "Inspect host details, filesystems, network interfaces, and supported ranges"}},
			"/hosts/{id}/metrics":           map[string]any{"get": map[string]any{"summary": "Downsampled host time-series metrics (15m, 1h, 24h, 7d)"}},
			"/services":                     map[string]any{"get": map[string]any{"summary": "List monitored services"}, "post": map[string]any{"summary": "Create SSRF-validated HTTP/HTTPS/TCP/DNS/TLS check"}},
			"/services/{id}":                map[string]any{"get": map[string]any{"summary": "Inspect service uptime and recent observations"}, "delete": map[string]any{"summary": "Remove service monitor"}},
			"/services/{id}/check-now":      map[string]any{"post": map[string]any{"summary": "Trigger immediate service check"}},
			"/containers":                   map[string]any{"get": map[string]any{"summary": "Inspect containers and truthful Docker Engine integration status"}},
			"/containers/{id}/action":       map[string]any{"post": map[string]any{"summary": "Execute confirmed allowlisted start/stop/restart container action"}},
			"/deployment-targets":           map[string]any{"get": map[string]any{"summary": "List deployment targets"}, "post": map[string]any{"summary": "Register deployment target"}},
			"/deployments":                  map[string]any{"get": map[string]any{"summary": "List deployment history"}, "post": map[string]any{"summary": "Queue controlled deployment with per-target lock and idempotency"}},
			"/deployments/{id}/rollback":    map[string]any{"post": map[string]any{"summary": "Roll back target to verified previous artifact"}},
			"/logs":                         map[string]any{"get": map[string]any{"summary": "Search and filter structured logs"}, "post": map[string]any{"summary": "Ingest structured log entry with automatic credential redaction"}},
			"/alerts/rules":                 map[string]any{"get": map[string]any{"summary": "List alert rules"}, "post": map[string]any{"summary": "Create alert rule with threshold, duration, cooldown, and optional auto-incident"}},
			"/alerts/events":                map[string]any{"get": map[string]any{"summary": "List persistent alert transitions"}},
			"/incidents":                    map[string]any{"get": map[string]any{"summary": "List incidents"}, "post": map[string]any{"summary": "Create incident"}},
			"/incidents/{id}":               map[string]any{"get": map[string]any{"summary": "Inspect incident details and timeline"}, "patch": map[string]any{"summary": "Update status, assignment, root cause, resolution, or postmortem"}},
			"/incidents/{id}/summarize":     map[string]any{"post": map[string]any{"summary": "Optional redacted AI incident summary when explicitly configured"}},
			"/insights":                     map[string]any{"get": map[string]any{"summary": "Deterministic operational insights distinguishing correlation from root cause"}},
			"/topology":                     map[string]any{"get": map[string]any{"summary": "Configured infrastructure topology graph"}},
			"/integrations":                 map[string]any{"get": map[string]any{"summary": "List integration connection states"}},
			"/integrations/github/overview": map[string]any{"get": map[string]any{"summary": "Read-only GitHub commits and workflow runs when connected"}},
			"/webhooks/github/{orgId}":      map[string]any{"post": map[string]any{"summary": "HMAC-SHA256 verified GitHub webhook receiver with delivery deduplication"}},
			"/audit":                        map[string]any{"get": map[string]any{"summary": "Organization-scoped immutable audit trail"}},
			"/backups":                      map[string]any{"get": map[string]any{"summary": "List backup records"}, "post": map[string]any{"summary": "Create consistent SQLite VACUUM INTO backup with SHA-256 verification"}},
		},
	}
	writeJSON(w, http.StatusOK, spec)
}

const apiDocsHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>InfraOS API Reference</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #F3F6F8; color: #172A32; margin: 0; padding: 2rem; line-height: 1.5; }
    .wrap { max-width: 960px; margin: 0 auto; background: #fff; border: 1px solid rgba(23,42,50,0.12); border-radius: 12px; padding: 2rem; }
    h1 { margin-top: 0; font-size: 1.75rem; }
    .meta { color: #586971; margin-bottom: 1.5rem; }
    .row { display: grid; grid-template-columns: 90px 280px 1fr; gap: 1rem; padding: 0.75rem 0; border-top: 1px solid rgba(23,42,50,0.08); align-items: baseline; }
    .method { font-family: ui-monospace, monospace; font-weight: 700; font-size: 0.75rem; text-transform: uppercase; color: #237C83; }
    .path { font-family: ui-monospace, monospace; font-size: 0.85rem; }
    @media (max-width: 720px) { .row { grid-template-columns: 1fr; gap: 0.25rem; } }
  </style>
</head>
<body>
  <main class="wrap">
    <h1>InfraOS Control Plane API v1</h1>
    <p class="meta">Interactive schema generated directly from <code>/api/v1/openapi.json</code>. Exposed in development mode only.</p>
    <div id="routes">Loading OpenAPI specification...</div>
  </main>
  <script>
    fetch('/api/v1/openapi.json').then(r => r.json()).then(spec => {
      const container = document.getElementById('routes');
      container.innerHTML = '';
      for (const [path, ops] of Object.entries(spec.paths || {})) {
        for (const [method, info] of Object.entries(ops)) {
          const row = document.createElement('div');
          row.className = 'row';
          row.innerHTML = '<span class="method">' + method + '</span><span class="path">/api/v1' + path + '</span><span>' + (info.summary || '') + '</span>';
          container.appendChild(row);
        }
      }
    }).catch(() => {
      document.getElementById('routes').textContent = 'Failed to load OpenAPI specification.';
    });
  </script>
</body>
</html>`

func (s *Server) handleAPIDocsHTML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(apiDocsHTML))
}
