"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { ServiceCheck, ServiceRecord } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatRelativeTime, formatShortTime } from "@/lib/format";

export default function ServiceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { session, canOperate } = useWorkspace();

  const detailQuery = useQuery({
    queryKey: ["service", id],
    queryFn: () => apiFetch<{ service: ServiceRecord; checks: ServiceCheck[] }>(`/api/v1/services/${id}`),
    enabled: Boolean(session && id),
  });

  if (detailQuery.isLoading) {
    return <StateView kind="loading" title="Loading service details" description="Reading check observations and latency series." />;
  }
  if (detailQuery.isError || !detailQuery.data) {
    return (
      <StateView
        kind="error"
        title="Service not found"
        description={(detailQuery.error as Error)?.message || "Could not load service."}
        action={<Link href="/workspace/services" className="infra-btn-secondary">Back to services</Link>}
      />
    );
  }

  const { service, checks } = detailQuery.data;
  const chartData = [...checks].reverse().map((c) => ({
    time: formatShortTime(c.checked_at),
    latency: c.latency_ms,
  }));

  async function handleRunNow() {
    await apiFetch(`/api/v1/services/${service.id}/check-now`, { method: "POST" });
    await queryClient.invalidateQueries({ queryKey: ["service", id] });
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <Link href="/workspace/services" className="text-xs text-[var(--text-secondary)] hover:underline">
            Service monitoring
          </Link>
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="text-2xl font-bold tracking-tight">{service.name}</h1>
            <StatusBadge status={service.status} />
            {service.is_synthetic ? <StatusBadge status="synthetic" label="Synthetic demo" /> : null}
          </div>
          <p className="font-mono text-xs text-[var(--text-secondary)]">
            {service.check_type.toUpperCase()} · {service.target} · every {service.interval_seconds}s
          </p>
        </div>
        {canOperate && !service.is_synthetic ? (
          <button type="button" onClick={handleRunNow} className="infra-btn-primary">
            Run check now
          </button>
        ) : null}
      </header>

      <section className="infra-surface grid gap-4 p-5 sm:grid-cols-4">
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Observed uptime</div>
          <div className="mt-1 font-mono text-xl font-bold">
            {service.uptime_percent !== null ? `${service.uptime_percent}%` : "No observations"}
          </div>
          <div className="text-xs text-[var(--text-secondary)]">{service.observations_count} total checks</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Latest latency</div>
          <div className="mt-1 font-mono text-xl font-bold">{service.last_latency_ms} ms</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Consecutive failures</div>
          <div className="mt-1 font-mono text-xl font-bold">{service.consecutive_failures}</div>
          <div className="text-xs text-[var(--text-secondary)]">Threshold {service.failure_threshold}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">TLS certificate expiry</div>
          <div className="mt-1 font-mono text-sm font-semibold">
            {service.tls_expires_at ? service.tls_expires_at.slice(0, 10) : "Not applicable"}
          </div>
        </div>
      </section>

      <section className="infra-surface p-5">
        <h2 className="mb-3 text-base font-semibold">Response time history (ms)</h2>
        {chartData.length === 0 ? (
          <p className="text-sm text-[var(--text-secondary)]">No observations recorded yet.</p>
        ) : (
          <div className="h-60 w-full">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="time" tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                <YAxis unit="ms" tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                <Tooltip />
                <Line type="monotone" dataKey="latency" name="Latency (ms)" stroke="#237C83" strokeWidth={2} dot={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        )}
      </section>

      <section className="infra-surface overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-3.5">
          <h2 className="text-base font-semibold">Recent probe observations</h2>
        </div>
        <div className="infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Time</th>
                <th scope="col">Result</th>
                <th scope="col">HTTP status</th>
                <th scope="col">Latency</th>
                <th scope="col">Details</th>
              </tr>
            </thead>
            <tbody>
              {checks.map((c) => (
                <tr key={c.id}>
                  <th scope="row" className="font-mono text-xs">{formatRelativeTime(c.checked_at)}</th>
                  <td data-label="Result"><StatusBadge status={c.status} /></td>
                  <td data-label="HTTP status" className="font-mono text-xs">{c.status_code || "-"}</td>
                  <td data-label="Latency" className="font-mono text-xs">{c.latency_ms} ms</td>
                  <td data-label="Details" className="text-xs text-[var(--text-secondary)]">
                    {c.failure_reason || (c.tls_days_remaining !== null && c.tls_days_remaining !== undefined ? `TLS valid ${c.tls_days_remaining}d` : "OK")}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
