"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { AuditEvent } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

export default function AuditPage() {
  const { session } = useWorkspace();
  const [resourceFilter, setResourceFilter] = useState("");

  const auditQuery = useQuery({
    queryKey: ["audit", session?.organization.id, resourceFilter],
    queryFn: () =>
      apiFetch<AuditEvent[]>(
        `/api/v1/audit${resourceFilter ? `?resource_type=${encodeURIComponent(resourceFilter)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Audit trail</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Immutable organization-scoped record of authentication, enrollment, deployment, RBAC, and backup actions.
          </p>
        </div>
        <select
          aria-label="Filter by resource type"
          value={resourceFilter}
          onChange={(e) => setResourceFilter(e.target.value)}
          className="infra-input w-auto"
        >
          <option value="">All resources</option>
          <option value="session">Sessions</option>
          <option value="host">Hosts</option>
          <option value="agent">Agents</option>
          <option value="service">Services</option>
          <option value="deployment">Deployments</option>
          <option value="incident">Incidents</option>
          <option value="backup">Backups</option>
        </select>
      </header>

      {auditQuery.isLoading ? (
        <StateView kind="loading" title="Loading audit trail" description="Reading recorded security and operational events." />
      ) : (auditQuery.data || []).length === 0 ? (
        <StateView kind="empty" title="No audit events found" description="Administrative and operational actions are recorded automatically." />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Timestamp</th>
                <th scope="col">Action</th>
                <th scope="col">Actor</th>
                <th scope="col">Resource</th>
                <th scope="col">Status</th>
                <th scope="col">IP</th>
              </tr>
            </thead>
            <tbody>
              {(auditQuery.data || []).map((ev) => (
                <tr key={ev.id}>
                  <th scope="row" className="font-mono text-xs">{ev.created_at}</th>
                  <td data-label="Action" className="font-mono text-xs font-semibold">{ev.action}</td>
                  <td data-label="Actor" className="font-mono text-xs">
                    {ev.actor_email || (ev.actor_agent_id ? `agent:${ev.actor_agent_id.slice(0, 8)}` : "system")}
                  </td>
                  <td data-label="Resource" className="font-mono text-xs">
                    {ev.resource_type}:{ev.resource_id.slice(0, 8)}
                  </td>
                  <td data-label="Status"><StatusBadge status={ev.status === "success" ? "ok" : "error"} label={ev.status} /></td>
                  <td data-label="IP" className="font-mono text-xs text-[var(--text-secondary)]">{ev.ip_address || "-"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
