# Changelog

All notable changes to InfraOS are documented in this file.

## [0.1.0] - 2026-11-25

### Added
- Go control plane (`cmd/api`), standalone background worker (`cmd/worker`), and cross-platform macOS/Linux telemetry agent (`cmd/agent`).
- Dialect-specific SQLite and PostgreSQL schema migrations (`database/migrations/`).
- Argon2id authentication, HttpOnly sessions, CSRF protection, login rate limiting, server-side RBAC, organization isolation, and immutable audit trail.
- Separate agent authentication with one-time expiring enrollment tokens, rotation, and immediate revocation.
- Real OS metric collection (CPU, cores, load, memory, mounted filesystems, counter-reset-safe network throughput, uptime, optional battery and process summary).
- SSRF-validated HTTP, HTTPS, TCP, DNS, and TLS service monitoring scheduler with observation-based uptime calculation.
- Optional Unix-socket Docker Engine inspection with read-only defaults and explicit mutation allowlists.
- Controlled deployment engine with atomic per-target SQL locking, idempotency keys, health verification, rollback, and clearly labeled simulation mode.
- Centralized structured logs with credential redaction, alert rules with cooldowns and auto-incident escalation, incident timeline workspace with postmortems, deterministic operational insights, optional disabled-by-default AI summarizer, GitHub API + HMAC-SHA256 webhooks, and interactive topology map.
- Original InfraOS SVG brand identity, Liquid Glass Next.js App Router frontend (9 public routes, 19 workspace routes), Cmd/Ctrl+K command palette, light/dark themes, and isolated synthetic demo mode.
- Dockerfiles, Docker Compose, Nginx + TLS template, Prometheus + alert rules, Terraform examples, SQLite/PostgreSQL backup and restore scripts, GitHub Actions workflows, 8 SVG architecture diagrams, and complete engineering documentation.
