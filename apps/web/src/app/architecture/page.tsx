import type { Metadata } from "next";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "Architecture",
  description: "InfraOS Go backend, SQLite/PostgreSQL persistence, real system agent, and Next.js frontend architecture.",
};

export default function ArchitecturePage() {
  return (
    <PublicShell>
      <div className="mx-auto max-w-5xl space-y-10 px-6 py-14">
        <header className="max-w-3xl space-y-4">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            System architecture
          </h1>
          <p className="text-base text-[var(--text-secondary)]">
            InfraOS separates telemetry collection, API persistence, background scheduling, and
            browser presentation behind explicit Go interfaces and versioned REST contracts.
          </p>
        </header>

        <section className="infra-surface p-6">
          <h2 className="text-lg font-semibold">Logical component flow</h2>
          <div className="mt-4 grid gap-4 md:grid-cols-3">
            <div className="infra-surface-subtle space-y-1.5 p-4">
              <div className="font-mono text-xs font-semibold text-[var(--accent)]">cmd/agent</div>
              <div className="text-sm font-semibold">Host Telemetry Agent</div>
              <p className="text-xs leading-relaxed text-[var(--text-secondary)]">
                Collects OS metrics with gopsutil and platform helpers, buffers samples in a bounded
                retry queue, and pushes over authenticated HTTP.
              </p>
            </div>
            <div className="infra-surface-subtle space-y-1.5 p-4">
              <div className="font-mono text-xs font-semibold text-[var(--accent)]">cmd/api + cmd/worker</div>
              <div className="text-sm font-semibold">Control Plane &amp; Scheduler</div>
              <p className="text-xs leading-relaxed text-[var(--text-secondary)]">
                Chi HTTP router, Argon2id sessions, RBAC, monitoring worker pool, alert evaluator,
                deployment engine, SSE broker, and Prometheus metrics.
              </p>
            </div>
            <div className="infra-surface-subtle space-y-1.5 p-4">
              <div className="font-mono text-xs font-semibold text-[var(--accent)]">apps/web</div>
              <div className="text-sm font-semibold">Liquid Glass Workspace</div>
              <p className="text-xs leading-relaxed text-[var(--text-secondary)]">
                Next.js App Router frontend with TanStack Query, SSE reconnect with polling
                fallback, Recharts visualizations, and interactive topology graph.
              </p>
            </div>
          </div>
        </section>

        <section className="infra-surface space-y-4 p-6">
          <h2 className="text-lg font-semibold">Request and concurrency lifecycle</h2>
          <ol className="space-y-3 text-sm text-[var(--text-secondary)]">
            <li>
              <strong className="text-[var(--text-primary)]">1. Ingestion:</strong> Authenticated
              agents submit JSON payloads via <code className="font-mono">POST /api/v1/agents/metrics</code>.
              Transactions persist metrics, refresh heartbeats, trigger alert rules, and broadcast
              an org-scoped SSE event.
            </li>
            <li>
              <strong className="text-[var(--text-primary)]">2. Monitoring scheduler:</strong> Every
              5 seconds, due checks enter a semaphore-bounded goroutine pool. Each probe validates
              DNS resolution and HTTP redirects against SSRF rules before dialing.
            </li>
            <li>
              <strong className="text-[var(--text-primary)]">3. Controlled deployments:</strong> A
              conditional SQL lock claims the target row atomically so overlapping rollouts return
              HTTP 409 Conflict instead of corrupting live state.
            </li>
            <li>
              <strong className="text-[var(--text-primary)]">4. Observability:</strong> InfraOS
              instruments itself via <code className="font-mono">/metrics</code>, structured{" "}
              <code className="font-mono">slog</code> JSON logs, <code className="font-mono">/healthz</code>,
              and <code className="font-mono">/readyz</code>.
            </li>
          </ol>
        </section>

        <div className="flex flex-wrap gap-3">
          <Link href="/security" className="infra-btn-primary">
            Inspect security model
          </Link>
          <Link href="/docs" className="infra-btn-secondary">
            Browse documentation
          </Link>
        </div>
      </div>
    </PublicShell>
  );
}
