"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Incident } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

const statuses: Array<Incident["status"]> = [
  "open",
  "investigating",
  "identified",
  "monitoring",
  "resolved",
];

export default function IncidentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const { session, canOperate } = useWorkspace();
  const [note, setNote] = useState("");
  const [rootCause, setRootCause] = useState("");
  const [resolution, setResolution] = useState("");
  const [postmortem, setPostmortem] = useState("");
  const [aiStatus, setAiStatus] = useState("");
  const [savedMsg, setSavedMsg] = useState("");

  const incQuery = useQuery({
    queryKey: ["incident", id],
    queryFn: async () => {
      const data = await apiFetch<Incident>(`/api/v1/incidents/${id}`);
      setRootCause(data.root_cause_notes || "");
      setResolution(data.resolution_summary || "");
      setPostmortem(data.postmortem_markdown || "");
      return data;
    },
    enabled: Boolean(session && id),
  });

  if (incQuery.isLoading) {
    return <StateView kind="loading" title="Loading incident" description="Reading timeline and investigation records." />;
  }
  if (incQuery.isError || !incQuery.data) {
    return (
      <StateView
        kind="error"
        title="Incident not found"
        description={(incQuery.error as Error)?.message || "Could not load incident."}
        action={<Link href="/workspace/incidents" className="infra-btn-secondary">Back to incidents</Link>}
      />
    );
  }

  const inc = incQuery.data;

  async function updateStatus(nextStatus: Incident["status"]) {
    await apiFetch(`/api/v1/incidents/${inc.id}`, {
      method: "PATCH",
      json: { status: nextStatus },
    });
    await queryClient.invalidateQueries({ queryKey: ["incident", id] });
    await queryClient.invalidateQueries({ queryKey: ["incidents"] });
  }

  async function handleAddNote(e: React.FormEvent) {
    e.preventDefault();
    if (!note.trim()) return;
    await apiFetch(`/api/v1/incidents/${inc.id}/timeline`, {
      method: "POST",
      json: { event_type: "note", message: note },
    });
    setNote("");
    await queryClient.invalidateQueries({ queryKey: ["incident", id] });
  }

  async function handleSavePostmortem(e: React.FormEvent) {
    e.preventDefault();
    await apiFetch(`/api/v1/incidents/${inc.id}`, {
      method: "PATCH",
      json: {
        root_cause_notes: rootCause,
        resolution_summary: resolution,
        postmortem_markdown: postmortem,
      },
    });
    setSavedMsg("Investigation notes and postmortem saved.");
    await queryClient.invalidateQueries({ queryKey: ["incident", id] });
  }

  async function handleSummarize() {
    setAiStatus("Requesting redacted advisory summary...");
    try {
      const res = await apiFetch<{ summary: string; advisory_note: string }>(`/api/v1/incidents/${inc.id}/summarize`, {
        method: "POST",
      });
      setAiStatus(`${res.summary} (${res.advisory_note})`);
      await queryClient.invalidateQueries({ queryKey: ["incident", id] });
    } catch (err) {
      setAiStatus(err instanceof APIError ? err.message : "AI summarization is disabled.");
    }
  }

  return (
    <div className="space-y-6">
      <header className="space-y-2">
        <Link href="/workspace/incidents" className="text-xs text-[var(--text-secondary)] hover:underline">
          Incidents
        </Link>
        <div className="flex flex-wrap items-center gap-2.5">
          <StatusBadge status={inc.severity} label={inc.severity.toUpperCase()} />
          <StatusBadge status={inc.status} />
          {inc.is_synthetic ? <StatusBadge status="synthetic" label="Synthetic demo" /> : null}
        </div>
        <h1 className="text-2xl font-bold tracking-tight">{inc.title}</h1>
        <p className="max-w-3xl text-sm text-[var(--text-secondary)]">{inc.description}</p>
      </header>

      {canOperate ? (
        <section aria-label="Incident lifecycle" className="infra-surface flex flex-wrap items-center justify-between gap-3 p-4">
          <div className="flex flex-wrap gap-2">
            {statuses.map((st) => (
              <button
                key={st}
                type="button"
                onClick={() => updateStatus(st)}
                className={
                  inc.status === st
                    ? "infra-btn-primary min-h-[34px] px-3 text-xs capitalize"
                    : "infra-btn-secondary min-h-[34px] px-3 text-xs capitalize"
                }
              >
                {st}
              </button>
            ))}
          </div>
          <button type="button" onClick={handleSummarize} className="infra-btn-secondary min-h-[34px] px-3 text-xs">
            Optional AI summary
          </button>
        </section>
      ) : null}

      {aiStatus ? (
        <div role="status" className="infra-surface-subtle p-4 text-xs text-[var(--text-primary)]">
          {aiStatus}
        </div>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[1.2fr_1fr]">
        {/* Operational Timeline */}
        <section className="infra-surface space-y-4 p-5">
          <h2 className="text-base font-semibold">Operational timeline</h2>
          <ol className="space-y-3 border-l-2 border-[var(--border)] pl-4">
            {(inc.timeline || []).map((te) => (
              <li key={te.id} className="space-y-1">
                <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]">
                  <span className="font-mono font-semibold text-[var(--text-primary)]">{te.event_type}</span>
                  {te.author_email ? <span>{te.author_email}</span> : null}
                  <span className="font-mono">{te.created_at}</span>
                </div>
                <p className="text-sm text-[var(--text-primary)]">{te.message}</p>
              </li>
            ))}
          </ol>

          {canOperate ? (
            <form onSubmit={handleAddNote} className="space-y-2 border-t border-[var(--border)] pt-4">
              <label htmlFor="timeline-note" className="block text-xs font-semibold text-[var(--text-secondary)]">
                Add investigation note
              </label>
              <div className="flex gap-2">
                <input
                  id="timeline-note"
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Record diagnostic finding or mitigation step"
                  className="infra-input"
                />
                <button type="submit" className="infra-btn-primary shrink-0">
                  Post note
                </button>
              </div>
            </form>
          ) : null}
        </section>

        {/* Investigation & Postmortem */}
        <form onSubmit={handleSavePostmortem} className="infra-surface space-y-4 p-5">
          <h2 className="text-base font-semibold">Resolution &amp; postmortem</h2>
          <div className="space-y-1.5">
            <label htmlFor="inc-rc" className="block text-xs font-medium text-[var(--text-secondary)]">
              Confirmed root cause notes
            </label>
            <textarea id="inc-rc" rows={3} value={rootCause} onChange={(e) => setRootCause(e.target.value)} disabled={!canOperate} className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="inc-res" className="block text-xs font-medium text-[var(--text-secondary)]">
              Resolution summary
            </label>
            <textarea id="inc-res" rows={3} value={resolution} onChange={(e) => setResolution(e.target.value)} disabled={!canOperate} className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="inc-pm" className="block text-xs font-medium text-[var(--text-secondary)]">
              Postmortem markdown
            </label>
            <textarea id="inc-pm" rows={5} value={postmortem} onChange={(e) => setPostmortem(e.target.value)} disabled={!canOperate} className="infra-input font-mono text-xs" />
          </div>
          {savedMsg ? <p role="status" className="text-xs text-[var(--status-ok)]">{savedMsg}</p> : null}
          {canOperate ? (
            <button type="submit" className="infra-btn-primary">Save documentation</button>
          ) : null}
        </form>
      </div>
    </div>
  );
}
