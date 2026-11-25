"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Incident } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";

export default function IncidentsPage() {
  const queryClient = useQueryClient();
  const { session, selectedEnvId, canOperate } = useWorkspace();
  const [statusFilter, setStatusFilter] = useState("");
  const [showForm, setShowForm] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [severity, setSeverity] = useState("sev3");
  const [errMsg, setErrMsg] = useState("");

  const incQuery = useQuery({
    queryKey: ["incidents", session?.organization.id, statusFilter],
    queryFn: () =>
      apiFetch<Incident[]>(
        `/api/v1/incidents${statusFilter ? `?status=${encodeURIComponent(statusFilter)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    setErrMsg("");
    try {
      await apiFetch("/api/v1/incidents", {
        method: "POST",
        json: { title, description, severity, environment_id: selectedEnvId },
      });
      setTitle("");
      setDescription("");
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ["incidents"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Could not open incident.");
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Incident workspace</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Track active incidents from detection through investigation, mitigation, and postmortem.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <select
            aria-label="Filter incidents by status"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
            className="infra-input w-auto"
          >
            <option value="">All statuses</option>
            <option value="open">Open</option>
            <option value="investigating">Investigating</option>
            <option value="identified">Identified</option>
            <option value="monitoring">Monitoring</option>
            <option value="resolved">Resolved</option>
          </select>
          {canOperate ? (
            <button type="button" onClick={() => setShowForm((v) => !v)} className="infra-btn-primary">
              {showForm ? "Cancel" : "Declare incident"}
            </button>
          ) : null}
        </div>
      </header>

      {showForm ? (
        <form onSubmit={handleCreate} className="infra-surface space-y-4 p-5">
          <div className="grid gap-4 md:grid-cols-[1fr_160px]">
            <div className="space-y-1.5">
              <label htmlFor="inc-title" className="block text-sm font-medium">Incident title</label>
              <input id="inc-title" value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="Describe operational impact" className="infra-input" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="inc-sev" className="block text-sm font-medium">Severity</label>
              <select id="inc-sev" value={severity} onChange={(e) => setSeverity(e.target.value)} className="infra-input">
                <option value="sev1">SEV1 (Critical)</option>
                <option value="sev2">SEV2 (Major)</option>
                <option value="sev3">SEV3 (Minor)</option>
                <option value="sev4">SEV4 (Low)</option>
              </select>
            </div>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="inc-desc" className="block text-sm font-medium">Summary &amp; observed symptoms</label>
            <textarea id="inc-desc" rows={3} value={description} onChange={(e) => setDescription(e.target.value)} className="infra-input" />
          </div>
          {errMsg ? <p role="alert" className="text-xs text-[var(--status-err)]">{errMsg}</p> : null}
          <button type="submit" className="infra-btn-primary">Open incident</button>
        </form>
      ) : null}

      {incQuery.isLoading ? (
        <StateView kind="loading" title="Loading incidents" description="Retrieving open and historical incidents." />
      ) : (incQuery.data || []).length === 0 ? (
        <StateView kind="empty" title="No incidents recorded" description="Incidents declared manually or opened by alert rules will appear here." />
      ) : (
        <div className="space-y-3">
          {(incQuery.data || []).map((inc) => (
            <Link
              key={inc.id}
              href={`/workspace/incidents/${inc.id}`}
              className="infra-surface block p-5 transition-colors hover:bg-[var(--surface-subtle)]"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={inc.severity} label={inc.severity.toUpperCase()} />
                    <StatusBadge status={inc.status} />
                    {inc.is_synthetic ? <StatusBadge status="synthetic" label="Synthetic demo" /> : null}
                  </div>
                  <h2 className="text-base font-semibold text-[var(--text-primary)]">{inc.title}</h2>
                  {inc.description ? (
                    <p className="max-w-3xl text-sm text-[var(--text-secondary)]">{inc.description}</p>
                  ) : null}
                </div>
                <div className="text-right font-mono text-xs text-[var(--text-secondary)]">
                  <div>Opened {formatRelativeTime(inc.opened_at)}</div>
                  {inc.assigned_email ? <div className="mt-1">{inc.assigned_email}</div> : null}
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}
