"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import type { Container, ContainerIntegrationStatus } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatBytes } from "@/lib/format";

export default function ContainersPage() {
  const { session } = useWorkspace();

  const containersQuery = useQuery({
    queryKey: ["containers", session?.organization.id],
    queryFn: () =>
      apiFetch<{ integration: ContainerIntegrationStatus; containers: Container[] }>("/api/v1/containers"),
    enabled: Boolean(session),
  });

  if (containersQuery.isLoading) {
    return <StateView kind="loading" title="Checking container integration" description="Inspecting configured Docker Engine status." />;
  }
  if (containersQuery.isError || !containersQuery.data) {
    return <StateView kind="error" title="Unable to load containers" description={(containersQuery.error as Error)?.message || "API request failed."} />;
  }

  const { integration, containers } = containersQuery.data;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Containers</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Optional Docker Engine inspection with read-only defaults and explicit mutation allowlists.
          </p>
        </div>
        <StatusBadge status={integration.mode === "connected" ? "connected" : integration.mode === "synthetic_demo" ? "synthetic" : "offline"} label={integration.mode.replace(/_/g, " ")} />
      </header>

      <section className="infra-surface space-y-2 p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">Docker integration boundary</h2>
          <span className="font-mono text-xs text-[var(--text-secondary)]">
            Policy: {integration.mutations_policy}
          </span>
        </div>
        <p className="text-sm text-[var(--text-secondary)]">{integration.message}</p>
        {integration.mode === "disabled" || integration.mode === "unavailable" ? (
          <pre className="mt-2 overflow-x-auto rounded-lg bg-[var(--surface-subtle)] p-3 font-mono text-xs">
{`INFRAOS_DOCKER_ENABLED=true
INFRAOS_DOCKER_SOCKET=/var/run/docker.sock
INFRAOS_DOCKER_ALLOW_MUTATIONS=false`}
          </pre>
        ) : null}
      </section>

      {containers.length === 0 ? (
        <StateView
          kind="empty"
          title="No live containers detected"
          description="InfraOS never fabricates live container entries when Docker is disabled or unavailable on the host."
        />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Container</th>
                <th scope="col">Image</th>
                <th scope="col">State &amp; health</th>
                <th scope="col">CPU / Memory</th>
                <th scope="col">Restarts</th>
                <th scope="col">Details</th>
              </tr>
            </thead>
            <tbody>
              {containers.map((c) => (
                <tr key={c.id}>
                  <th scope="row">
                    <Link href={`/workspace/containers/${c.id}`} className="font-mono font-semibold hover:underline">
                      {c.name}
                    </Link>
                    {c.is_synthetic ? <div className="mt-1"><StatusBadge status="synthetic" label="Synthetic demo" /></div> : null}
                  </th>
                  <td data-label="Image" className="font-mono text-xs">{c.image}</td>
                  <td data-label="State">
                    <div className="flex flex-wrap gap-1.5">
                      <StatusBadge status={c.state} />
                      {c.health_status && c.health_status !== "none" ? <StatusBadge status={c.health_status} /> : null}
                    </div>
                  </td>
                  <td data-label="Resources" className="font-mono text-xs">
                    {c.cpu_percent.toFixed(1)}% · {formatBytes(c.mem_usage_bytes)}
                  </td>
                  <td data-label="Restarts" className="font-mono text-xs">{c.restart_count}</td>
                  <td data-label="Details">
                    <Link href={`/workspace/containers/${c.id}`} className="infra-btn-secondary min-h-[32px] px-2.5 text-xs">
                      Logs &amp; inspect
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
