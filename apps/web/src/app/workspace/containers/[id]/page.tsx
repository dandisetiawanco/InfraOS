"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Container, ContainerIntegrationStatus } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatBytes } from "@/lib/format";

export default function ContainerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { session, canManage } = useWorkspace();
  const [actionMsg, setActionMsg] = useState("");
  const [actionErr, setActionErr] = useState("");

  const detailQuery = useQuery({
    queryKey: ["container", id],
    queryFn: () =>
      apiFetch<{ integration: ContainerIntegrationStatus; container: Container }>(`/api/v1/containers/${id}`),
    enabled: Boolean(session && id),
  });

  const logsQuery = useQuery({
    queryKey: ["container", id, "logs"],
    queryFn: () => apiFetch<{ lines: string[] }>(`/api/v1/containers/${id}/logs`),
    enabled: Boolean(session && id),
  });

  if (detailQuery.isLoading) {
    return <StateView kind="loading" title="Loading container" description="Fetching runtime state and recent logs." />;
  }
  if (detailQuery.isError || !detailQuery.data) {
    return (
      <StateView
        kind="error"
        title="Container not found"
        description={(detailQuery.error as Error)?.message || "Could not load container."}
        action={<Link href="/workspace/containers" className="infra-btn-secondary">Back to containers</Link>}
      />
    );
  }

  const { container, integration } = detailQuery.data;

  async function runAction(action: "start" | "stop" | "restart") {
    setActionMsg("");
    setActionErr("");
    try {
      await apiFetch(`/api/v1/containers/${container.id}/action`, {
        method: "POST",
        json: { action, confirmed: true },
      });
      setActionMsg(`Executed ${action} on ${container.name}.`);
      await queryClient.invalidateQueries({ queryKey: ["container", id] });
    } catch (err) {
      setActionErr(err instanceof APIError ? err.message : "Action rejected by policy.");
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <Link href="/workspace/containers" className="text-xs text-[var(--text-secondary)] hover:underline">
            Containers
          </Link>
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="font-mono text-2xl font-bold">{container.name}</h1>
            <StatusBadge status={container.state} />
            {container.is_synthetic ? <StatusBadge status="synthetic" label="Synthetic demo" /> : null}
          </div>
          <p className="font-mono text-xs text-[var(--text-secondary)]">
            {container.image} · ID {container.external_id.slice(0, 16)}
          </p>
        </div>

        {canManage && container.allow_mutations && !container.is_synthetic ? (
          <div className="flex gap-2">
            <button type="button" onClick={() => runAction("restart")} className="infra-btn-secondary">
              Restart
            </button>
            <button type="button" onClick={() => runAction("stop")} className="infra-btn-secondary">
              Stop
            </button>
          </div>
        ) : (
          <span className="text-xs text-[var(--text-secondary)]">Read-only inspection ({integration.mutations_policy})</span>
        )}
      </header>

      {actionMsg ? <div role="status" className="rounded-xl bg-[var(--status-ok-bg)] p-3 text-sm text-[var(--status-ok)]">{actionMsg}</div> : null}
      {actionErr ? <div role="alert" className="rounded-xl bg-[var(--status-err-bg)] p-3 text-sm text-[var(--status-err)]">{actionErr}</div> : null}

      <section className="infra-surface grid gap-4 p-5 sm:grid-cols-4">
        <div>
          <div className="text-xs text-[var(--text-secondary)]">CPU usage</div>
          <div className="mt-1 font-mono text-lg font-bold">{container.cpu_percent.toFixed(1)}%</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Memory</div>
          <div className="mt-1 font-mono text-lg font-bold">{formatBytes(container.mem_usage_bytes)}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Networks</div>
          <div className="mt-1 font-mono text-xs">{container.networks.join(", ") || "default"}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Restart count</div>
          <div className="mt-1 font-mono text-lg font-bold">{container.restart_count}</div>
        </div>
      </section>

      <section className="infra-surface overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-3.5">
          <h2 className="text-base font-semibold">Recent container logs</h2>
        </div>
        <pre className="max-h-96 overflow-auto p-5 font-mono text-xs leading-relaxed">
          {logsQuery.isLoading
            ? "Loading logs..."
            : (logsQuery.data?.lines || ["No log output available."]).join("\n")}
        </pre>
      </section>
    </div>
  );
}
