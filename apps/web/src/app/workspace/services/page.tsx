"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ServiceRecord } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";

export default function ServicesPage() {
  const queryClient = useQueryClient();
  const { session, selectedEnvId, environments, canOperate } = useWorkspace();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [checkType, setCheckType] = useState("http");
  const [target, setTarget] = useState("");
  const [envId, setEnvId] = useState("");
  const [intervalSeconds, setIntervalSeconds] = useState(30);
  const [expectedStatus, setExpectedStatus] = useState(200);
  const [formErr, setFormErr] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const servicesQuery = useQuery({
    queryKey: ["services", session?.organization.id, selectedEnvId],
    queryFn: () =>
      apiFetch<ServiceRecord[]>(
        `/api/v1/services${selectedEnvId ? `?environment_id=${encodeURIComponent(selectedEnvId)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setFormErr("");
    setSubmitting(true);
    try {
      await apiFetch("/api/v1/services", {
        method: "POST",
        json: {
          name,
          check_type: checkType,
          target,
          environment_id: envId,
          interval_seconds: Number(intervalSeconds),
          expected_status: Number(expectedStatus),
        },
      });
      setName("");
      setTarget("");
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ["services"] });
    } catch (err) {
      setFormErr(err instanceof APIError ? err.message : "Failed to create service monitor.");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleRunCheck(id: string) {
    try {
      await apiFetch(`/api/v1/services/${id}/check-now`, { method: "POST" });
      await queryClient.invalidateQueries({ queryKey: ["services"] });
    } catch {
      // Errors surfaced in service detail row
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Service monitoring</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Scheduled HTTP, HTTPS, TCP, DNS, and TLS checks validated against SSRF network policies.
          </p>
        </div>
        {canOperate ? (
          <button
            type="button"
            disabled={session?.organization.is_demo}
            onClick={() => setShowForm((v) => !v)}
            className="infra-btn-primary"
          >
            {session?.organization.is_demo
              ? "Target creation disabled in demo"
              : showForm
                ? "Cancel"
                : "Add monitored service"}
          </button>
        ) : null}
      </header>

      {showForm ? (
        <form onSubmit={handleCreate} className="infra-surface grid gap-4 p-5 md:grid-cols-2">
          <div className="space-y-1.5">
            <label htmlFor="svc-name" className="block text-sm font-medium">Service name</label>
            <input id="svc-name" value={name} onChange={(e) => setName(e.target.value)} required placeholder="Local API Health" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="svc-type" className="block text-sm font-medium">Check protocol</label>
            <select id="svc-type" value={checkType} onChange={(e) => setCheckType(e.target.value)} className="infra-input">
              <option value="http">HTTP</option>
              <option value="https">HTTPS</option>
              <option value="tcp">TCP</option>
              <option value="dns">DNS</option>
              <option value="tls">TLS certificate</option>
            </select>
          </div>
          <div className="space-y-1.5 md:col-span-2">
            <label htmlFor="svc-target" className="block text-sm font-medium">Authorized endpoint or host:port</label>
            <input id="svc-target" value={target} onChange={(e) => setTarget(e.target.value)} required placeholder="http://127.0.0.1:8080/healthz" className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="svc-env" className="block text-sm font-medium">Environment</label>
            <select id="svc-env" value={envId} onChange={(e) => setEnvId(e.target.value)} className="infra-input">
              <option value="">Unassigned</option>
              {environments.map((env) => (
                <option key={env.id} value={env.id}>{env.name}</option>
              ))}
            </select>
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <label htmlFor="svc-int" className="block text-sm font-medium">Interval (s)</label>
              <input id="svc-int" type="number" min={10} max={3600} value={intervalSeconds} onChange={(e) => setIntervalSeconds(Number(e.target.value))} className="infra-input font-mono" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="svc-code" className="block text-sm font-medium">Expected HTTP</label>
              <input id="svc-code" type="number" value={expectedStatus} onChange={(e) => setExpectedStatus(Number(e.target.value))} className="infra-input font-mono" />
            </div>
          </div>
          {formErr ? (
            <div role="alert" className="md:col-span-2 rounded-lg bg-[var(--status-err-bg)] px-3.5 py-2 text-xs text-[var(--status-err)]">
              {formErr}
            </div>
          ) : null}
          <div className="md:col-span-2">
            <button type="submit" disabled={submitting} className="infra-btn-primary">
              {submitting ? "Validating target..." : "Save service monitor"}
            </button>
          </div>
        </form>
      ) : null}

      {servicesQuery.isLoading ? (
        <StateView kind="loading" title="Loading monitored services" description="Fetching check configurations and observed availability." />
      ) : (servicesQuery.data || []).length === 0 ? (
        <StateView
          kind="empty"
          title="No monitored services configured"
          description="Add an authorized HTTP, HTTPS, TCP, DNS, or TLS check. Availability percentages are recorded strictly after observations run."
        />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Service</th>
                <th scope="col">Protocol</th>
                <th scope="col">Status</th>
                <th scope="col">Observed uptime</th>
                <th scope="col">Latency</th>
                <th scope="col">Last checked</th>
                <th scope="col">Actions</th>
              </tr>
            </thead>
            <tbody>
              {(servicesQuery.data || []).map((svc) => (
                <tr key={svc.id}>
                  <th scope="row">
                    <Link href={`/workspace/services/${svc.id}`} className="font-semibold hover:underline">
                      {svc.name}
                    </Link>
                    <div className="font-mono text-xs font-normal text-[var(--text-secondary)]">
                      {svc.target}
                    </div>
                  </th>
                  <td data-label="Protocol" className="font-mono text-xs uppercase">{svc.check_type}</td>
                  <td data-label="Status"><StatusBadge status={svc.status} /></td>
                  <td data-label="Observed uptime" className="font-mono text-xs">
                    {svc.uptime_percent !== null
                      ? `${svc.uptime_percent}% (${svc.observations_count} checks)`
                      : "Pending first check"}
                  </td>
                  <td data-label="Latency" className="font-mono text-xs">{svc.last_latency_ms} ms</td>
                  <td data-label="Last checked" className="font-mono text-xs text-[var(--text-secondary)]">
                    {formatRelativeTime(svc.last_checked_at)}
                  </td>
                  <td data-label="Actions">
                    <div className="flex gap-2">
                      <Link href={`/workspace/services/${svc.id}`} className="infra-btn-secondary min-h-[32px] px-2.5 text-xs">
                        Inspect
                      </Link>
                      {canOperate && !svc.is_synthetic ? (
                        <button
                          type="button"
                          onClick={() => handleRunCheck(svc.id)}
                          className="infra-btn-secondary min-h-[32px] px-2.5 text-xs"
                        >
                          Run check
                        </button>
                      ) : null}
                    </div>
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
