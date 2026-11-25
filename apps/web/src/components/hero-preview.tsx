"use client";

import { useState } from "react";
import Link from "next/link";
import { Activity, GitBranch, HardDrive, ShieldAlert } from "lucide-react";
import { StatusBadge } from "./status-badge";

type PreviewTab = "hosts" | "services" | "deployments" | "incidents";

const tabs: Array<{ id: PreviewTab; label: string; icon: typeof Activity }> = [
  { id: "hosts", label: "Host telemetry", icon: HardDrive },
  { id: "services", label: "Service probes", icon: Activity },
  { id: "deployments", label: "Deployments", icon: GitBranch },
  { id: "incidents", label: "Incidents", icon: ShieldAlert },
];

export function HeroWorkspacePreview() {
  const [active, setActive] = useState<PreviewTab>("hosts");

  return (
    <section
      aria-label="Interactive InfraOS workspace preview"
      className="infra-surface overflow-hidden shadow-sm"
    >
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--border)] bg-[var(--surface-subtle)] px-4 py-3">
        <div className="flex items-center gap-2 text-xs font-medium text-[var(--text-secondary)]">
          <span className="font-mono text-[var(--text-primary)]">workspace</span>
          <span>Interactive preview</span>
          <StatusBadge status="synthetic" label="Synthetic sample" />
        </div>
        <div role="tablist" aria-label="Workspace preview modules" className="flex flex-wrap gap-1.5">
          {tabs.map(({ id, label, icon: Icon }) => {
            const selected = active === id;
            return (
              <button
                key={id}
                type="button"
                role="tab"
                aria-selected={selected}
                onClick={() => setActive(id)}
                className={`inline-flex min-h-[34px] items-center gap-1.5 rounded-lg px-3 text-xs font-medium transition-colors ${
                  selected
                    ? "bg-[var(--accent)] text-[var(--accent-contrast)]"
                    : "bg-[var(--surface)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
                }`}
              >
                <Icon className="h-3.5 w-3.5" aria-hidden="true" />
                <span>{label}</span>
              </button>
            );
          })}
        </div>
      </div>

      <div className="grid gap-4 p-5 lg:grid-cols-[200px_1fr]">
        <aside className="infra-glass-nav hidden flex-col justify-between rounded-xl p-3 lg:flex">
          <div className="space-y-1 text-xs">
            <div className="px-2 py-1 font-semibold text-[var(--text-secondary)]">Operations</div>
            {tabs.map(({ id, label }) => (
              <button
                key={id}
                type="button"
                onClick={() => setActive(id)}
                className={`flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-left font-medium ${
                  active === id
                    ? "bg-[var(--accent-soft)] text-[var(--text-primary)]"
                    : "text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)]"
                }`}
              >
                <span>{label}</span>
              </button>
            ))}
          </div>
          <div className="border-t border-[var(--border)] pt-3 text-[11px] text-[var(--text-secondary)]">
            Live workspace connects to your enrolled macOS or Linux hosts.
          </div>
        </aside>

        <div className="space-y-4">
          {active === "hosts" ? (
            <div className="space-y-4">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <div>
                  <h3 className="text-sm font-semibold text-[var(--text-primary)]">
                    Connected hosts (sample topology)
                  </h3>
                  <p className="text-xs text-[var(--text-secondary)]">
                    Real collector reports CPU, memory, APFS or ext4 capacity, and network throughput deltas.
                  </p>
                </div>
                <Link href="/demo" className="text-xs font-semibold text-[var(--accent)] underline">
                  Open live sandbox
                </Link>
              </div>
              <div className="grid gap-3 sm:grid-cols-3">
                {[
                  { name: "edge-api-01", os: "linux/amd64", cpu: "42.1%", mem: "64.0%", status: "online" },
                  { name: "worker-pool-02", os: "linux/amd64", cpu: "68.4%", mem: "84.2%", status: "online" },
                  { name: "stg-app-01", os: "linux/amd64", cpu: "18.2%", mem: "48.1%", status: "stale" },
                ].map((host) => (
                  <div key={host.name} className="infra-surface-subtle space-y-2 p-3.5">
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-xs font-semibold">{host.name}</span>
                      <StatusBadge status={host.status} />
                    </div>
                    <div className="text-xs text-[var(--text-secondary)]">{host.os}</div>
                    <div className="grid grid-cols-2 gap-2 border-t border-[var(--border)] pt-2 font-mono text-xs">
                      <div>
                        <span className="block text-[11px] text-[var(--text-secondary)]">CPU</span>
                        <span>{host.cpu}</span>
                      </div>
                      <div>
                        <span className="block text-[11px] text-[var(--text-secondary)]">Memory</span>
                        <span>{host.mem}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {active === "services" ? (
            <div className="space-y-3">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold">Scheduled health checks (SSRF validated)</h3>
                <span className="font-mono text-xs text-[var(--text-secondary)]">HTTP · HTTPS · TCP · DNS · TLS</span>
              </div>
              <div className="divide-y divide-[var(--border)] rounded-xl border border-[var(--border)]">
                {[
                  { name: "Edge API Gateway", type: "HTTPS", latency: "34 ms", status: "healthy", note: "TLS valid 62d" },
                  { name: "Job Queue Processor", type: "HTTP", latency: "468 ms", status: "degraded", note: "Latency above baseline" },
                  { name: "Staging DNS Resolver", type: "DNS", latency: "0 ms", status: "down", note: "3 consecutive failures" },
                ].map((svc) => (
                  <div key={svc.name} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3 text-xs">
                    <div>
                      <div className="font-medium text-[var(--text-primary)]">{svc.name}</div>
                      <div className="text-[var(--text-secondary)]">{svc.note}</div>
                    </div>
                    <div className="flex items-center gap-3 font-mono">
                      <span>{svc.type}</span>
                      <span>{svc.latency}</span>
                      <StatusBadge status={svc.status} />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ) : null}

          {active === "deployments" ? (
            <div className="space-y-3">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold">Controlled release pipeline with target locking</h3>
                <span className="font-mono text-xs text-[var(--text-secondary)]">Rollback verified</span>
              </div>
              <div className="space-y-2 rounded-xl border border-[var(--border)] p-4 text-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="font-mono font-semibold">Edge API Production · v2.14.2</span>
                  <StatusBadge status="succeeded" />
                </div>
                <p className="text-[var(--text-secondary)]">
                  Commit <span className="font-mono">c7d31e50a4</span> verified via post-deploy HTTP probe. Previous artifact <span className="font-mono">v2.14.1</span> retained for rollback.
                </p>
                <ol className="grid gap-2 pt-2 sm:grid-cols-5">
                  {["Queued", "Preparing", "Building", "Deploying", "Verifying"].map((step) => (
                    <li key={step} className="rounded-lg bg-[var(--surface-subtle)] px-2.5 py-2 text-center font-medium">
                      {step}
                    </li>
                  ))}
                </ol>
              </div>
            </div>
          ) : null}

          {active === "incidents" ? (
            <div className="space-y-3">
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <h3 className="text-sm font-semibold">Incident timeline & deterministic correlation</h3>
                <StatusBadge status="investigating" />
              </div>
              <div className="space-y-2.5 rounded-xl border border-[var(--border)] p-4 text-xs">
                <div className="font-semibold text-[var(--text-primary)]">
                  Staging DNS Resolver check timeouts
                </div>
                <p className="text-[var(--text-secondary)]">
                  Alert rule opened a deduplicated SEV2 incident after 3 consecutive check failures. Temporal correlation to recent staging deployment is flagged separately from confirmed root cause.
                </p>
                <div className="flex flex-wrap gap-2 pt-1 font-mono text-[11px] text-[var(--text-secondary)]">
                  <span className="rounded bg-[var(--surface-subtle)] px-2 py-1">Open</span>
                  <span className="rounded bg-[var(--accent-soft)] px-2 py-1 text-[var(--text-primary)]">Investigating</span>
                  <span className="rounded bg-[var(--surface-subtle)] px-2 py-1">Identified</span>
                  <span className="rounded bg-[var(--surface-subtle)] px-2 py-1">Monitoring</span>
                  <span className="rounded bg-[var(--surface-subtle)] px-2 py-1">Resolved</span>
                </div>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}
