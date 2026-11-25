# Automated Testing Strategy

InfraOS prioritizes behavioral and security verification over superficial line coverage metrics. No coverage percentages are fabricated.

## Go Backend Tests (`go test -race -count=1 ./...`)

1. **`internal/security`** (6 tests): Argon2id password hashing and rejection of short/wrong passwords, AES-256-GCM secret encryption and masking, SSRF blocking of cloud metadata (`169.254.169.254`, `100.100.100.200`), RFC1918 private networks, non-allowlisted loopback, and demo isolation, GitHub HMAC-SHA256 signature verification, credential redaction in logs, and per-IP rate limiting.
2. **`pkg/collector`** (2 tests): Counter-reset-safe network throughput calculation and real live macOS/Linux OS metric collection (hostname, CPU cores, CPU %, memory, disk).
3. **`internal/api`** (5 integration tests): Full HTTP flow testing registration, login, session revocation, RBAC enforcement (viewer blocked with HTTP 403), cross-organization isolation (Org B receives HTTP 404 for Org A resources), one-time agent enrollment + reuse rejection + metric ingestion + revocation, real loopback HTTP service check + SSRF rejection + observed-only uptime, deployment lifecycle + idempotency deduplication + atomic target lock (HTTP 409) + rollback to verified artifact, alert firing/recovery + auto-incident creation, GitHub HMAC webhook deduplication, SQLite `VACUUM INTO` backup checksum, and public demo sandbox isolation.

## Frontend Unit Tests (`pnpm --filter @infraos/web test`)

Vitest + React Testing Library verifying formatting helpers, status/freshness components, loading/empty/error UI states, `<meta name="color-scheme">` theme persistence, and interactive landing preview tabs.

## Playwright End-to-End & Responsive Verification (`apps/web/e2e/infraos.spec.ts`)

Authored for live Go API + Next.js verification (deferred from initial lightweight publication checks; not yet executed in full):
- Landing page rendering, interactive preview tabs, zero horizontal overflow across 390px, 768px, 1024px, 1440px, and 1920px viewports.
- Isolated demo launch, workspace overview, host details, service monitoring, containers, deployments, logs, alerts, incident timeline, topology graph, Cmd+K command palette, dark mode, mobile layout, and real local macOS host monitoring.
- Zero console errors across all tested routes.
