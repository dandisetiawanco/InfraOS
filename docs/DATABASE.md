# Database Design and Migrations

InfraOS supports **SQLite** (pure-Go `modernc.org/sqlite`, WAL mode, `PRAGMA foreign_keys = ON`) for lightweight local development and **PostgreSQL 16** (`pgx/v5`) for infrastructure and production profiles.

## Dialect Compatibility

SQLite and PostgreSQL differ in boolean types, auto-increment primary keys, and timestamp types (`TEXT` vs `TIMESTAMPTZ`). InfraOS maintains explicit dialect-specific SQL migrations:
- `database/migrations/sqlite/001_initial.sql`
- `database/migrations/postgres/001_initial.sql`

At runtime, `internal/database/database.go` tracks applied versions in `schema_migrations` inside transactions and rebinds `?` placeholders to `$1, $2, ...` when running on PostgreSQL.

## Core Schema Groups

- **Identity & tenancy**: `users`, `organizations`, `organization_memberships`, `sessions`
- **Organization hierarchy**: `projects`, `environments`, `topology_links`
- **Hosts & agents**: `hosts`, `agents`, `agent_enrollment_tokens`, `agent_credentials`, `host_metrics`
- **Service monitoring**: `services`, `service_checks`
- **Containers**: `containers`, `container_snapshots`
- **Deployments**: `deployment_targets`, `deployment_artifacts`, `deployments`, `deployment_events`
- **Logs, alerts, incidents**: `log_sources`, `log_entries`, `alert_rules`, `alert_events`, `incidents`, `incident_timelines`, `notifications`
- **Governance & operations**: `audit_events`, `integration_settings`, `webhook_deliveries`, `backup_records`

## Consistency, Retention, and Isolation

- Every query filters explicitly by `organization_id` derived from the authenticated session or verified agent credential.
- Sensitive multi-row mutations (registration, agent enrollment, credential rotation, deployment lock acquisition) execute inside `db.WithTx`.
- Retention pruning (`db.PruneRetention`) removes expired `host_metrics`, `log_entries`, `service_checks`, and stale `sessions` on a 6-hour schedule.
- Raw user passwords and raw session/agent tokens are never persisted in plaintext.
