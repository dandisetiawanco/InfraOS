# REST API Reference

InfraOS exposes a versioned JSON REST API under `/api/v1` with consistent response envelopes:

```json
{ "data": { ... }, "meta": { "total": 42 } }
```

Errors include a machine-readable code, human message, and `request_id`:

```json
{ "error": { "code": "forbidden", "message": "Role \"viewer\" does not have \"manage_hosts\" permission.", "request_id": "..." } }
```

## Interactive Development Documentation

When `INFRAOS_EXPOSE_API_DOCS=true` (default in lightweight development, disable in public production unless explicitly intended):
- OpenAPI 3.1 JSON: `GET /api/v1/openapi.json`
- Interactive route viewer: `GET /api/docs`

## Endpoint Groups

- **Authentication**: `POST /auth/register`, `POST /auth/login`, `POST /auth/logout`, `POST /auth/demo`, `GET /auth/me`, `POST /auth/switch-org`
- **Agent protocol (separate `Authorization: Agent <token>`)**: `POST /agents/enroll`, `POST /agents/metrics`
- **Hosts & agents**: `GET /hosts`, `GET/PATCH /hosts/{id}`, `GET /hosts/{id}/metrics?range=15m|1h|24h|7d`, `POST /agents/enrollment-tokens`, `POST /agents/{id}/rotate`, `POST /agents/{id}/revoke`
- **Service monitoring**: `GET/POST /services`, `GET/DELETE /services/{id}`, `GET /services/{id}/checks`, `POST /services/{id}/check-now`
- **Containers**: `GET /containers`, `GET /containers/{id}`, `GET /containers/{id}/logs`, `POST /containers/{id}/action`
- **Deployments**: `GET/POST /deployment-targets`, `GET/POST /deployments`, `GET /deployments/{id}`, `POST /deployments/{id}/rollback`
- **Logs**: `GET/POST /logs`, `GET /logs/export`
- **Alerts & notifications**: `GET/POST /alerts/rules`, `DELETE /alerts/rules/{id}`, `GET /alerts/events`, `GET /notifications`, `POST /notifications/read-all`
- **Incidents & insights**: `GET/POST /incidents`, `GET/PATCH /incidents/{id}`, `POST /incidents/{id}/timeline`, `POST /incidents/{id}/summarize`, `GET /insights`
- **Topology, integrations, audit, backups**: `GET /topology`, `POST /topology/links`, `GET /integrations`, `PUT /integrations/{provider}`, `GET /integrations/github/overview`, `POST /webhooks/github/{orgId}`, `GET /audit`, `GET/POST /backups`
- **Live SSE stream**: `GET /events/stream`
