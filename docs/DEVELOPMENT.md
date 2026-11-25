# Development Guide

## Repository Layout

- `cmd/api`: Core REST + SSE control plane server.
- `cmd/agent`: Cross-platform macOS and Linux telemetry agent.
- `cmd/worker`: Standalone background worker for multi-process profiles.
- `internal/`: Domain packages (`auth`, `hosts`, `monitoring`, `containers`, `deployments`, `logs`, `alerts`, `incidents`, `intelligence`, `integrations`, `topology`, `audit`, `database`, `security`, `queue`, `sse`, `config`, `api`).
- `pkg/`: Reusable Go packages (`collector`, `contracts`, `telemetry`).
- `database/migrations/`: Separate SQLite and PostgreSQL versioned SQL migrations embedded at compile time.
- `apps/web/`: Next.js App Router frontend with strict TypeScript and Tailwind CSS v4.
- `packages/contracts/`: Shared TypeScript API response contracts.
- `infrastructure/`: Dockerfiles, Nginx configs, Prometheus rules, sample workload, and Terraform modules.

## Useful Commands

| Command | Action |
|---|---|
| `make setup` | Tidy Go modules, install pnpm workspace packages, create local dirs |
| `make dev` | Build Go binaries and start API + frontend with port conflict checks |
| `make agent` | Run the local Go agent in background |
| `make test` | Run Go race tests and frontend Vitest suite |
| `make lint` | Check `gofmt`, `go vet`, ESLint, and TypeScript `tsc --noEmit` |
| `make build` | Build Go binaries (`api`, `agent`, `worker`) and Next.js production bundle |
| `make backup-sqlite` | Create a consistent `VACUUM INTO` snapshot with SHA-256 digest |
| `make test-restore-sqlite` | Verify snapshot integrity in a temporary directory without touching live DB |
| `make stop` | Stop only PIDs recorded under `.infraos/pids/` |

## Environment Profiles

Do not run the lightweight profile and infrastructure Docker Compose profile simultaneously on overlapping ports. Configure `.env` from `.env.example`; never commit `.env` or credential files.
