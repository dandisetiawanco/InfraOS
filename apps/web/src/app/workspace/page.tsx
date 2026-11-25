"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import type { OverviewData } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { DataFreshnessTag, StateView, StatusBadge } from "@/components/status-badge";
import { formatBps, formatRelativeTime } from "@/lib/format";

export default function WorkspaceOverviewPage() {
  const { session, selectedEnvId } = useWorkspace();

  const overviewQuery = useQuery({
    queryKey: ["overview", session?.organization.id, selectedEnvId],
    queryFn: () =>
      apiFetch<OverviewData>(
        `/api/v1/overview${selectedEnvId ? `?environment_id=${encodeURIComponent(selectedEnvId)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  if (overviewQuery.isLoading) {
    return (
      <StateView
        kind="loading"
        title="Loading infrastructure overview"
        description="Collecting latest host heartbeats, service observations, deployments, and operational insights."
      />
    );
  }

  if (overviewQuery.isError || !overviewQuery.data) {
    return (
      <StateView
        kind="error"
        title="Failed to load infrastructure overview"
        description={(overviewQuery.error as Error)?.message || "Unable to reach the InfraOS API."}
        action={
          <button
            type="button"
            onClick={() => overviewQuery.refetch()}
            className="infra-btn-primary"
          >
            Retry request
          </button>
        }
      />
    );
  }

  const data = overviewQuery.data;
  const s = data.summary;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Infrastructure overview</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Operational health across connected hosts, monitored endpoints, releases, and active incidents.
          </p>
        </div>
        <DataFreshnessTag
          source={data.data_source}
          updatedAt={data.latest_at ? `Updated ${formatRelativeTime(data.latest_at)}` : undefined}
        />
      </header>

      {/* Asymmetric primary operational strip */}
      <section aria-label="Operational summary" className="grid gap-4 lg:grid-cols-[1.5fr_1fr]">
        <div className="infra-surface grid gap-4 p-5 sm:grid-cols-2 xl:grid-cols-4">
          <div className="space-y-1">
            <div className="text-xs font-medium text-[var(--text-secondary)]">Connected hosts</div>
            <div className="font-mono text-2xl font-bold">{s.hosts_total}</div>
            <div className="text-xs text-[var(--text-secondary)]">
              {s.hosts_online} online · {s.hosts_stale} stale · {s.hosts_offline} offline
            </div>
          </div>

          <div className="space-y-1">
            <div className="text-xs font-medium text-[var(--text-secondary)]">Service availability</div>
            <div className="font-mono text-2xl font-bold">
              {s.service_availability_pct !== null ? `${s.service_availability_pct}%` : "No checks"}
            </div>
            <div className="text-xs text-[var(--text-secondary)]">
              {s.services_healthy} healthy · {s.services_degraded} degraded · {s.services_down} down
            </div>
          </div>

          <div className="space-y-1">
            <div className="text-xs font-medium text-[var(--text-secondary)]">Open incidents</div>
            <div className="font-mono text-2xl font-bold">{s.open_incidents}</div>
            <div className="text-xs text-[var(--text-secondary)]">
              {data.recent_alerts.filter((a) => a.state === "firing").length} firing alerts
            </div>
          </div>

          <div className="space-y-1">
            <div className="text-xs font-medium text-[var(--text-secondary)]">Aggregate network</div>
            <div className="font-mono text-base font-bold">
              Rx {formatBps(s.total_net_rx_bps)}
            </div>
            <div className="font-mono text-xs text-[var(--text-secondary)]">
              Tx {formatBps(s.total_net_tx_bps)}
            </div>
          </div>
        </div>

        <div className="infra-surface grid grid-cols-3 divide-x divide-[var(--border)] p-5">
          <div className="pr-3">
            <div className="text-xs text-[var(--text-secondary)]">Avg CPU</div>
            <div className="mt-1 font-mono text-xl font-bold">
              {s.avg_cpu_percent !== null ? `${s.avg_cpu_percent}%` : "N/A"}
            </div>
          </div>
          <div className="px-3">
            <div className="text-xs text-[var(--text-secondary)]">Avg Memory</div>
            <div className="mt-1 font-mono text-xl font-bold">
              {s.avg_mem_percent !== null ? `${s.avg_mem_percent}%` : "N/A"}
            </div>
          </div>
          <div className="pl-3">
            <div className="text-xs text-[var(--text-secondary)]">Avg Storage</div>
            <div className="mt-1 font-mono text-xl font-bold">
              {s.avg_disk_percent !== null ? `${s.avg_disk_percent}%` : "N/A"}
            </div>
          </div>
        </div>
      </section>

      {/* Deterministic Operational Insights */}
      {data.insights.length > 0 ? (
        <section aria-label="Operational insights" className="infra-surface p-5">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 className="text-base font-semibold">Deterministic operational insights</h2>
              <p className="text-xs text-[var(--text-secondary)]">
                Computed directly from stored metrics and checks. Correlations are separated from confirmed root cause.
              </p>
            </div>
          </div>
          <div className="space-y-2.5">
            {data.insights.map((ins) => (
              <div
                key={ins.id}
                className="infra-surface-subtle flex flex-wrap items-start justify-between gap-3 p-3.5"
              >
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={ins.severity} />
                    <span className="text-sm font-semibold">{ins.title}</span>
                    {ins.correlation_only ? (
                      <StatusBadge status="info" label="Correlation only" />
                    ) : null}
                  </div>
                  <p className="text-xs text-[var(--text-secondary)]">{ins.detail}</p>
                </div>
                <div className="font-mono text-xs text-[var(--text-secondary)]">
                  {ins.evidence[0]}
                </div>
              </div>
            ))}
          </div>
        </section>
      ) : null}

      {/* Hosts & Services split */}
      <div className="grid gap-6 xl:grid-cols-[1.25fr_1fr]">
        <section className="infra-surface overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3.5">
            <h2 className="text-base font-semibold">Hosts</h2>
            <Link href="/workspace/hosts" className="text-xs font-semibold text-[var(--accent)] underline">
              Manage hosts
            </Link>
          </div>
          {data.hosts.length === 0 ? (
            <div className="p-6 text-sm text-[var(--text-secondary)]">
              No hosts enrolled yet. Open{" "}
              <Link href="/workspace/hosts" className="underline">
                Hosts
              </Link>{" "}
              to generate a one-time enrollment token for your macOS or Linux machine.
            </div>
          ) : (
            <div className="infra-table-wrap">
              <table className="infra-table infra-table-stack">
                <thead>
                  <tr>
                    <th scope="col">Host</th>
                    <th scope="col">Status</th>
                    <th scope="col">CPU</th>
                    <th scope="col">Memory</th>
                    <th scope="col">Storage</th>
                  </tr>
                </thead>
                <tbody>
                  {data.hosts.map((h) => (
                    <tr key={h.id}>
                      <th scope="row">
                        <Link
                          href={`/workspace/hosts/${h.id}`}
                          className="font-semibold text-[var(--text-primary)] hover:underline"
                        >
                          {h.display_name}
                        </Link>
                        <div className="font-mono text-xs font-normal text-[var(--text-secondary)]">
                          {h.os}/{h.arch} · {h.ip_address}
                        </div>
                      </th>
                      <td data-label="Status">
                        <div className="flex flex-wrap gap-1.5">
                          <StatusBadge status={h.status} />
                          {h.is_synthetic ? <StatusBadge status="synthetic" label="Demo" /> : null}
                        </div>
                      </td>
                      <td data-label="CPU" className="font-mono">
                        {h.latest_metric ? `${h.latest_metric.cpu_percent.toFixed(1)}%` : "N/A"}
                      </td>
                      <td data-label="Memory" className="font-mono">
                        {h.latest_metric ? `${h.latest_metric.mem_percent.toFixed(1)}%` : "N/A"}
                      </td>
                      <td data-label="Storage" className="font-mono">
                        {h.latest_metric ? `${h.latest_metric.disk_percent.toFixed(1)}%` : "N/A"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="infra-surface overflow-hidden">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-5 py-3.5">
            <h2 className="text-base font-semibold">Recent deployments &amp; incidents</h2>
            <Link href="/workspace/deployments" className="text-xs font-semibold text-[var(--accent)] underline">
              All deployments
            </Link>
          </div>
          <div className="divide-y divide-[var(--border)]">
            {data.recent_deployments.length === 0 && data.open_incident_list.length === 0 ? (
              <div className="p-6 text-sm text-[var(--text-secondary)]">
                No deployments or incidents recorded in this workspace yet.
              </div>
            ) : (
              <>
                {data.open_incident_list
                  .filter((inc) => inc.status !== "resolved")
                  .slice(0, 3)
                  .map((inc) => (
                    <Link
                      key={inc.id}
                      href={`/workspace/incidents/${inc.id}`}
                      className="flex items-center justify-between gap-3 px-5 py-3.5 hover:bg-[var(--surface-subtle)]"
                    >
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold">{inc.title}</div>
                        <div className="text-xs text-[var(--text-secondary)]">
                          Incident opened {formatRelativeTime(inc.opened_at)}
                        </div>
                      </div>
                      <StatusBadge status={inc.severity} />
                    </Link>
                  ))}

                {data.recent_deployments.slice(0, 5).map((dep) => (
                  <Link
                    key={dep.id}
                    href={`/workspace/deployments/${dep.id}`}
                    className="flex items-center justify-between gap-3 px-5 py-3.5 hover:bg-[var(--surface-subtle)]"
                  >
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium">
                        {dep.target_name || "Target"}{" "}
                        <span className="font-mono text-xs text-[var(--accent)]">{dep.version}</span>
                      </div>
                      <div className="font-mono text-xs text-[var(--text-secondary)]">
                        {dep.commit_sha.slice(0, 10)} · {formatRelativeTime(dep.created_at)}
                        {dep.is_simulation ? " · simulation" : ""}
                      </div>
                    </div>
                    <StatusBadge status={dep.status} />
                  </Link>
                ))}
              </>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
