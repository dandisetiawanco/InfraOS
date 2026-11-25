# Isolated Demonstration Mode

InfraOS includes a safe public demonstration mode accessible at `/demo` (`POST /api/v1/auth/demo`).

## How Isolation Works

1. Launching the demo provisions a dedicated, isolated organization marked `is_demo = true` with its own temporary session cookie and operator account.
2. Every seeded record (hosts, 3-hour metric series, services, checks, containers, simulation targets, deployments, logs, alert rules, alert events, and an active incident) is explicitly marked `is_synthetic = true` in the database and visibly labeled in the UI.
3. Server-side guards block demo organizations from:
   - Issuing real agent enrollment tokens (`HTTP 403`)
   - Accessing the host Docker socket (`containers` switches to `synthetic_demo` mode)
   - Submitting external or allowlisted local monitoring probes or webhooks (`HTTP 400`)
   - Running non-simulation deployment adapters (`HTTP 400`)
   - Creating or inspecting live database backups (`HTTP 403`)
   - Reading any other organization's or demo session's data

To monitor your real macOS or Linux host, register an authenticated local account at `/register` instead of using `/demo`.
