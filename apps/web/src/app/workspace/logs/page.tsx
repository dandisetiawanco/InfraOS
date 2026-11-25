"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { LogEntry } from "@infraos/contracts";
import { API_BASE, apiFetch, apiFetchWithMeta } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

export default function LogsPage() {
  const queryClient = useQueryClient();
  const { session, selectedEnvId, canOperate } = useWorkspace();
  const [search, setSearch] = useState("");
  const [severity, setSeverity] = useState("");
  const [range, setRange] = useState("24h");
  const [offset, setOffset] = useState(0);
  const [ingestMsg, setIngestMsg] = useState("");
  const [ingestSev, setIngestSev] = useState("info");
  const [ingestSrc, setIngestSrc] = useState("application");
  const [showIngest, setShowIngest] = useState(false);

  const params = new URLSearchParams({ limit: "50", offset: String(offset) });
  if (search.trim()) params.set("q", search.trim());
  if (severity) params.set("severity", severity);
  if (range) params.set("range", range);
  if (selectedEnvId) params.set("environment_id", selectedEnvId);

  const logsQuery = useQuery({
    queryKey: ["logs", session?.organization.id, params.toString()],
    queryFn: () => apiFetchWithMeta<LogEntry[], { total: number }>(`/api/v1/logs?${params.toString()}`),
    enabled: Boolean(session),
  });

  async function handleIngest(e: React.FormEvent) {
    e.preventDefault();
    if (!ingestMsg.trim()) return;
    await apiFetch("/api/v1/logs", {
      method: "POST",
      json: {
        severity: ingestSev,
        source_name: ingestSrc,
        message: ingestMsg,
        environment_id: selectedEnvId,
      },
    });
    setIngestMsg("");
    setShowIngest(false);
    await queryClient.invalidateQueries({ queryKey: ["logs"] });
  }

  const entries = logsQuery.data?.data || [];
  const total = logsQuery.data?.meta?.total || 0;

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Centralized logs</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Structured JSON log entries with automatic credential redaction, correlation IDs, and bounded retention.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <a
            href={`${API_BASE}/api/v1/logs/export?${params.toString()}`}
            className="infra-btn-secondary"
          >
            Export CSV
          </a>
          {canOperate ? (
            <button
              type="button"
              onClick={() => setShowIngest((v) => !v)}
              className="infra-btn-primary"
            >
              {showIngest ? "Cancel" : "Ingest log entry"}
            </button>
          ) : null}
        </div>
      </header>

      {showIngest ? (
        <form onSubmit={handleIngest} className="infra-surface grid gap-3 p-5 md:grid-cols-[140px_180px_1fr_auto] md:items-end">
          <div className="space-y-1">
            <label htmlFor="log-sev" className="block text-xs font-medium">Severity</label>
            <select id="log-sev" value={ingestSev} onChange={(e) => setIngestSev(e.target.value)} className="infra-input">
              <option value="debug">debug</option>
              <option value="info">info</option>
              <option value="warn">warn</option>
              <option value="error">error</option>
            </select>
          </div>
          <div className="space-y-1">
            <label htmlFor="log-src" className="block text-xs font-medium">Source</label>
            <input id="log-src" value={ingestSrc} onChange={(e) => setIngestSrc(e.target.value)} className="infra-input font-mono" />
          </div>
          <div className="space-y-1">
            <label htmlFor="log-msg" className="block text-xs font-medium">Message (secrets auto-redacted)</label>
            <input id="log-msg" value={ingestMsg} onChange={(e) => setIngestMsg(e.target.value)} required placeholder="Write structured log event" className="infra-input font-mono" />
          </div>
          <button type="submit" className="infra-btn-primary">Submit</button>
        </form>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-[1fr_150px_150px]">
        <label htmlFor="log-search" className="visually-hidden">Search logs</label>
        <input
          id="log-search"
          type="search"
          value={search}
          onChange={(e) => {
            setOffset(0);
            setSearch(e.target.value);
          }}
          placeholder="Search message, source, or correlation ID..."
          className="infra-input"
        />
        <select
          aria-label="Filter by severity"
          value={severity}
          onChange={(e) => {
            setOffset(0);
            setSeverity(e.target.value);
          }}
          className="infra-input"
        >
          <option value="">All severities</option>
          <option value="debug">Debug</option>
          <option value="info">Info</option>
          <option value="warn">Warn</option>
          <option value="error">Error</option>
          <option value="fatal">Fatal</option>
        </select>
        <select
          aria-label="Time range"
          value={range}
          onChange={(e) => {
            setOffset(0);
            setRange(e.target.value);
          }}
          className="infra-input"
        >
          <option value="15m">Last 15 minutes</option>
          <option value="1h">Last hour</option>
          <option value="24h">Last 24 hours</option>
          <option value="7d">Last 7 days</option>
          <option value="">All retained</option>
        </select>
      </div>

      {logsQuery.isLoading ? (
        <StateView kind="loading" title="Querying structured logs" description="Loading matching log entries." />
      ) : entries.length === 0 ? (
        <StateView kind="empty" title="No logs match current filters" description="Widen the time range or ingest a structured log entry." />
      ) : (
        <div className="infra-surface overflow-hidden">
          <div className="divide-y divide-[var(--border)]">
            {entries.map((entry) => (
              <div key={entry.id} className="space-y-1.5 px-4 py-3 font-mono text-xs">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={entry.severity} />
                    <span className="font-semibold text-[var(--text-primary)]">{entry.source_name}</span>
                    {entry.host_name ? <span className="text-[var(--text-secondary)]">host={entry.host_name}</span> : null}
                    {entry.correlation_id ? <span className="text-[var(--text-secondary)]">corr={entry.correlation_id.slice(0, 14)}</span> : null}
                    {entry.is_synthetic ? <StatusBadge status="synthetic" label="Demo" /> : null}
                  </div>
                  <span className="text-[var(--text-secondary)]">{entry.timestamp}</span>
                </div>
                <p className="break-words text-[var(--text-primary)]">{entry.message}</p>
              </div>
            ))}
          </div>

          <div className="flex items-center justify-between border-t border-[var(--border)] px-4 py-3 text-xs text-[var(--text-secondary)]">
            <span>
              Showing {entries.length} of {total} entries
            </span>
            <div className="flex gap-2">
              <button
                type="button"
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - 50))}
                className="infra-btn-secondary min-h-[32px] px-3 text-xs"
              >
                Previous
              </button>
              <button
                type="button"
                disabled={offset + 50 >= total}
                onClick={() => setOffset(offset + 50)}
                className="infra-btn-secondary min-h-[32px] px-3 text-xs"
              >
                Next
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
