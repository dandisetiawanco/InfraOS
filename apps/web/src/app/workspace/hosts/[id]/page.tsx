"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import type { Host, MetricPoint } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { DataFreshnessTag, StateView, StatusBadge } from "@/components/status-badge";
import { formatBps, formatBytes, formatRelativeTime, formatShortTime, formatUptime } from "@/lib/format";

export default function HostDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { session, canManage } = useWorkspace();
  const [range, setRange] = useState("1h");
  const [actionMsg, setActionMsg] = useState("");
  const [actionErr, setActionErr] = useState("");

  const hostQuery = useQuery({
    queryKey: ["host", id],
    queryFn: () => apiFetch<Host>(`/api/v1/hosts/${id}`),
    enabled: Boolean(session && id),
  });

  const metricsQuery = useQuery({
    queryKey: ["host", id, "metrics", range],
    queryFn: () => apiFetch<{ range: string; points: MetricPoint[] }>(`/api/v1/hosts/${id}/metrics?range=${range}`),
    enabled: Boolean(session && id),
  });

  if (hostQuery.isLoading) {
    return <StateView kind="loading" title="Loading host details" description="Fetching telemetry and hardware inventory." />;
  }
  if (hostQuery.isError || !hostQuery.data) {
    return (
      <StateView
        kind="error"
        title="Host not found"
        description={(hostQuery.error as Error)?.message || "Could not load host details."}
        action={<Link href="/workspace/hosts" className="infra-btn-secondary">Back to hosts</Link>}
      />
    );
  }

  const host = hostQuery.data;
  const m = host.latest_metric;
  const points = (metricsQuery.data?.points || []).map((pt) => ({
    ...pt,
    timeLabel: formatShortTime(pt.recorded_at),
    rxKBps: Math.round((pt.net_rx_bps / 1024) * 10) / 10,
    txKBps: Math.round((pt.net_tx_bps / 1024) * 10) / 10,
  }));

  const availableRanges = host.supported_ranges && host.supported_ranges.length > 0 ? host.supported_ranges : ["15m", "1h"];

  async function handleRevokeAgent() {
    if (!host.agent_id) return;
    setActionErr("");
    setActionMsg("");
    try {
      await apiFetch(`/api/v1/agents/${host.agent_id}/revoke`, { method: "POST" });
      setActionMsg("Agent credentials revoked immediately.");
      await queryClient.invalidateQueries({ queryKey: ["host", id] });
      await queryClient.invalidateQueries({ queryKey: ["hosts"] });
    } catch (err) {
      setActionErr(err instanceof APIError ? err.message : "Revocation failed.");
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <Link href="/workspace/hosts" className="text-xs font-medium text-[var(--text-secondary)] hover:underline">
            Hosts directory
          </Link>
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="text-2xl font-bold tracking-tight">{host.display_name}</h1>
            <StatusBadge status={host.status} />
            <DataFreshnessTag source={host.freshness} />
          </div>
          <p className="font-mono text-xs text-[var(--text-secondary)]">
            {host.hostname} · {host.os}/{host.arch} · kernel {host.kernel_version || "unknown"} · {host.ip_address}
          </p>
        </div>

        {canManage && host.agent_id && host.agent_status === "active" && !host.is_synthetic ? (
          <button type="button" onClick={handleRevokeAgent} className="infra-btn-secondary">
            Revoke agent credential
          </button>
        ) : null}
      </div>

      {actionMsg ? (
        <div role="status" className="rounded-xl bg-[var(--status-ok-bg)] px-4 py-2.5 text-sm text-[var(--status-ok)]">
          {actionMsg}
        </div>
      ) : null}
      {actionErr ? (
        <div role="alert" className="rounded-xl bg-[var(--status-err-bg)] px-4 py-2.5 text-sm text-[var(--status-err)]">
          {actionErr}
        </div>
      ) : null}

      {/* Real-time resource readings */}
      <section className="infra-surface grid gap-4 p-5 sm:grid-cols-2 lg:grid-cols-5">
        <div>
          <div className="text-xs text-[var(--text-secondary)]">CPU utilization</div>
          <div className="mt-1 font-mono text-xl font-bold">{m ? `${m.cpu_percent.toFixed(1)}%` : "Unavailable"}</div>
          <div className="font-mono text-xs text-[var(--text-secondary)]">
            {m ? `${m.cpu_cores} cores · load ${m.load_1m.toFixed(2)}` : ""}
          </div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Memory</div>
          <div className="mt-1 font-mono text-xl font-bold">{m ? `${m.mem_percent.toFixed(1)}%` : "Unavailable"}</div>
          <div className="font-mono text-xs text-[var(--text-secondary)]">
            {m ? `${formatBytes(m.mem_used_bytes)} / ${formatBytes(m.mem_total_bytes)}` : ""}
          </div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Primary storage</div>
          <div className="mt-1 font-mono text-xl font-bold">{m ? `${m.disk_percent.toFixed(1)}%` : "Unavailable"}</div>
          <div className="font-mono text-xs text-[var(--text-secondary)]">
            {m ? `${formatBytes(m.disk_used_bytes)} / ${formatBytes(m.disk_total_bytes)}` : ""}
          </div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Network throughput</div>
          <div className="mt-1 font-mono text-sm font-bold">{m ? `Rx ${formatBps(m.net_rx_bps)}` : "Unavailable"}</div>
          <div className="font-mono text-xs text-[var(--text-secondary)]">{m ? `Tx ${formatBps(m.net_tx_bps)}` : ""}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Uptime &amp; heartbeat</div>
          <div className="mt-1 font-mono text-base font-bold">{m ? formatUptime(m.uptime_seconds) : "Unavailable"}</div>
          <div className="font-mono text-xs text-[var(--text-secondary)]">
            Seen {formatRelativeTime(host.last_heartbeat_at)}
          </div>
        </div>
      </section>

      {/* Optional host telemetry (only shown when genuinely exposed by OS) */}
      {host.optional && (host.optional.battery || host.optional.cpu_temperature_c || host.optional.processes) ? (
        <section className="infra-surface flex flex-wrap gap-6 px-5 py-3.5 text-xs">
          {host.optional.processes ? (
            <div>
              <span className="text-[var(--text-secondary)]">Active OS processes: </span>
              <span className="font-mono font-semibold">{host.optional.processes.total_count}</span>
            </div>
          ) : null}
          {host.optional.battery ? (
            <div>
              <span className="text-[var(--text-secondary)]">Battery: </span>
              <span className="font-mono font-semibold">
                {host.optional.battery.percent}% ({host.optional.battery.state})
              </span>
            </div>
          ) : null}
          {host.optional.cpu_temperature_c !== undefined ? (
            <div>
              <span className="text-[var(--text-secondary)]">Thermal zone: </span>
              <span className="font-mono font-semibold">{host.optional.cpu_temperature_c}°C</span>
            </div>
          ) : null}
        </section>
      ) : null}

      {/* Historical range selector + charts */}
      <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-base font-semibold">CPU and memory utilization over time</h2>
          <div className="flex gap-1.5" role="group" aria-label="Historical time range">
            {availableRanges.map((r) => (
              <button
                key={r}
                type="button"
                onClick={() => setRange(r)}
                className={range === r ? "infra-btn-primary min-h-[34px] px-3 text-xs" : "infra-btn-secondary min-h-[34px] px-3 text-xs"}
              >
                {r}
              </button>
            ))}
          </div>
        </div>

        {points.length === 0 ? (
          <StateView kind="empty" title="No metrics in selected window" description="Wait for the agent to transmit samples or choose another supported range." />
        ) : (
          <div className="grid gap-4 lg:grid-cols-2">
            <div className="infra-surface p-4">
              <h3 className="mb-3 text-sm font-semibold">CPU &amp; memory percentage ({range})</h3>
              <div className="h-60 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={points}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="timeLabel" tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                    <YAxis domain={[0, 100]} unit="%" tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                    <Tooltip />
                    <Area type="monotone" dataKey="cpu_percent" name="CPU %" stroke="#237C83" fill="#237C83" fillOpacity={0.18} strokeWidth={2} />
                    <Area type="monotone" dataKey="mem_percent" name="Memory %" stroke="#586971" fill="#586971" fillOpacity={0.12} strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>

            <div className="infra-surface p-4">
              <h3 className="mb-3 text-sm font-semibold">Network throughput KiB/s ({range})</h3>
              <div className="h-60 w-full">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={points}>
                    <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                    <XAxis dataKey="timeLabel" tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                    <YAxis tick={{ fontSize: 11, fill: "var(--text-secondary)" }} />
                    <Tooltip />
                    <Area type="monotone" dataKey="rxKBps" name="Rx KiB/s" stroke="#237C83" fill="#237C83" fillOpacity={0.18} strokeWidth={2} />
                    <Area type="monotone" dataKey="txKBps" name="Tx KiB/s" stroke="#92400E" fill="#92400E" fillOpacity={0.1} strokeWidth={2} />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>
          </div>
        )}
      </section>

      {/* Filesystems & Network Interfaces */}
      <div className="grid gap-6 lg:grid-cols-2">
        <section className="infra-surface overflow-hidden">
          <div className="border-b border-[var(--border)] px-5 py-3.5">
            <h2 className="text-base font-semibold">Mounted filesystems</h2>
          </div>
          {(host.filesystems || []).length === 0 ? (
            <p className="p-5 text-sm text-[var(--text-secondary)]">No filesystem inventory reported yet.</p>
          ) : (
            <div className="infra-table-wrap">
              <table className="infra-table infra-table-stack">
                <thead>
                  <tr>
                    <th scope="col">Mount</th>
                    <th scope="col">Type</th>
                    <th scope="col">Used / Total</th>
                    <th scope="col">Usage</th>
                  </tr>
                </thead>
                <tbody>
                  {(host.filesystems || []).map((fs) => (
                    <tr key={fs.mount_point}>
                      <th scope="row" className="font-mono text-xs">{fs.mount_point}</th>
                      <td data-label="Type" className="font-mono text-xs">{fs.fs_type}</td>
                      <td data-label="Capacity" className="font-mono text-xs">
                        {formatBytes(fs.used_bytes)} / {formatBytes(fs.total_bytes)}
                      </td>
                      <td data-label="Usage" className="font-mono text-xs">{fs.used_percent.toFixed(1)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="infra-surface overflow-hidden">
          <div className="border-b border-[var(--border)] px-5 py-3.5">
            <h2 className="text-base font-semibold">Network interfaces</h2>
          </div>
          {(host.interfaces || []).length === 0 ? (
            <p className="p-5 text-sm text-[var(--text-secondary)]">No network interfaces reported yet.</p>
          ) : (
            <div className="infra-table-wrap">
              <table className="infra-table infra-table-stack">
                <thead>
                  <tr>
                    <th scope="col">Interface</th>
                    <th scope="col">Received</th>
                    <th scope="col">Transmitted</th>
                  </tr>
                </thead>
                <tbody>
                  {(host.interfaces || []).slice(0, 10).map((ni) => (
                    <tr key={ni.name}>
                      <th scope="row" className="font-mono text-xs">
                        {ni.name}
                        {ni.addresses && ni.addresses[0] ? (
                          <div className="text-[11px] font-normal text-[var(--text-secondary)]">{ni.addresses[0]}</div>
                        ) : null}
                      </th>
                      <td data-label="Received" className="font-mono text-xs">{formatBytes(ni.rx_bytes)}</td>
                      <td data-label="Transmitted" className="font-mono text-xs">{formatBytes(ni.tx_bytes)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
