# Getting Started with InfraOS

InfraOS supports three operational profiles so you can run locally on an 8 GB laptop without containers or deploy with PostgreSQL, Redis, Nginx, and Prometheus on an authorized Linux host.

## Prerequisites (Lightweight Profile)

- Go 1.24 or newer
- Node.js 22+ and pnpm
- SQLite CLI (optional, for shell backup verification scripts; the Go API uses pure-Go `modernc.org/sqlite`)

Docker Desktop, PostgreSQL, Redis, and Kubernetes are **not** required for the lightweight profile.

## Quick Start

1. Install dependencies and prepare local directories:

```bash
make setup
```

2. Start the lightweight local stack (Go API + SQLite + Next.js web):

```bash
make dev
```

`scripts/dev.sh` checks ports `3000` and `8080`. If port `3000` is already occupied by another local application, it automatically selects port `3001` without terminating existing processes.

3. Open the application:
- Landing page: `http://localhost:3000` (or `http://localhost:3001` if 3000 was occupied)
- API health check: `http://127.0.0.1:8080/healthz`
- Interactive API docs (dev mode): `http://127.0.0.1:8080/api/docs`

4. Explore either mode:
- **Isolated Synthetic Demo**: Visit `/demo` and click **Launch isolated demo workspace**.
- **Real Local Infrastructure Mode**: Register an owner account at `/register`, open **Hosts -> Enroll host agent**, generate a one-time enrollment token, and run:

```bash
INFRAOS_AGENT_ENROLL_TOKEN="<token>" \
INFRAOS_AGENT_API_URL="http://127.0.0.1:8080" \
./bin/infraos-agent
```

5. Stop only InfraOS-managed processes:

```bash
make stop
```
