# Roadmap and Verification Status

## Implemented and Verified Locally (Lightweight Profile)
- Go control plane API (`cmd/api`), standalone worker (`cmd/worker`), and real macOS/Linux host agent (`cmd/agent`).
- SQLite (WAL) database with embedded migrations and transactional `VACUUM INTO` backup support (backup/restore shell scripts authored and pending dedicated execution).
- Argon2id auth, CSRF, rate limiting, RBAC, organization isolation, per-agent credential enrollment/rotation/revocation.
- SSRF-hardened HTTP, HTTPS, TCP, DNS, and TLS service monitoring scheduler.
- Controlled deployments with per-target SQL locking, idempotency keys, health verification, and rollback.
- Structured credential-redacting logs, anti-storm alert rules, incident workspace with postmortems, deterministic insights, interactive topology graph, GitHub API + HMAC webhooks, and audit trail.
- Original SVG brand identity, Liquid Glass Next.js workspace (all 19 routes), public product site (9 routes), isolated synthetic demo, light/dark themes, and automated Go + Vitest verification (extended Playwright E2E suite authored in `apps/web/e2e/infraos.spec.ts` and deferred for later milestone/CI).

## Configured and Pending External Infrastructure Verification
- PostgreSQL 16 + Redis 7 Docker Compose infrastructure stack (`docker-compose.yml` validated in GitHub Actions CI when Docker daemon is present).
- Terraform Linux provisioning (`infrastructure/terraform/` validated via `terraform fmt`/`validate` in GitHub Actions CI).
- Production Nginx TLS termination with real ACME certificates on an authorized Linux host.
- External Prometheus and OpenTelemetry collector scraping.

## Planned Future Enhancements
- Redis-backed distributed worker queue adapter alongside the in-process worker pool.
- WebAuthn / Passkey and TOTP multi-factor authentication.
- Multi-region active-probe agent distribution.
- Connecting authorized external workloads and production hosts once deployed and configured.
