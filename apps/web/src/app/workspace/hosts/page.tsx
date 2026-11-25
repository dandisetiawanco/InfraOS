"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import type { Host } from "@infraos/contracts";
import { API_BASE, apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";

interface EnrollTokenResp {
  id: string;
  label: string;
  token: string;
  expires_at: string;
}

export default function HostsPage() {
  const { session, selectedEnvId, environments, canManage } = useWorkspace();
  const [search, setSearch] = useState("");
  const [enrollLabel, setEnrollLabel] = useState("Local Workstation");
  const [enrollEnvId, setEnrollEnvId] = useState("");
  const [tokenResult, setTokenResult] = useState<EnrollTokenResp | null>(null);
  const [enrollError, setEnrollError] = useState("");
  const [issuing, setIssuing] = useState(false);
  const dialogRef = useRef<HTMLDialogElement>(null);

  const hostsQuery = useQuery({
    queryKey: ["hosts", session?.organization.id, selectedEnvId, search],
    queryFn: () => {
      const params = new URLSearchParams();
      if (selectedEnvId) params.set("environment_id", selectedEnvId);
      if (search.trim()) params.set("q", search.trim());
      return apiFetch<Host[]>(`/api/v1/hosts?${params.toString()}`);
    },
    enabled: Boolean(session),
  });

  async function handleGenerateToken(e: React.FormEvent) {
    e.preventDefault();
    setEnrollError("");
    setIssuing(true);
    try {
      const res = await apiFetch<EnrollTokenResp>("/api/v1/agents/enrollment-tokens", {
        method: "POST",
        json: { label: enrollLabel, environment_id: enrollEnvId, ttl_minutes: 30 },
      });
      setTokenResult(res);
    } catch (err) {
      setEnrollError(err instanceof APIError ? err.message : "Failed to create enrollment token.");
    } finally {
      setIssuing(false);
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Hosts &amp; agents</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Inspect enrolled macOS and Linux hosts, heartbeat freshness, resource usage, and agent credentials.
          </p>
        </div>
        {canManage ? (
          <button
            type="button"
            disabled={session?.organization.is_demo}
            onClick={() => {
              setTokenResult(null);
              setEnrollError("");
              dialogRef.current?.showModal();
            }}
            className="infra-btn-primary"
          >
            {session?.organization.is_demo ? "Enrollment disabled in demo" : "Enroll host agent"}
          </button>
        ) : null}
      </header>

      <div className="flex flex-wrap items-center gap-3">
        <label htmlFor="host-search" className="visually-hidden">
          Search hosts
        </label>
        <input
          id="host-search"
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by hostname, display name, IP, or tag..."
          className="infra-input max-w-md"
        />
      </div>

      {hostsQuery.isLoading ? (
        <StateView kind="loading" title="Loading hosts" description="Fetching host inventory and latest metrics." />
      ) : hostsQuery.isError ? (
        <StateView
          kind="error"
          title="Could not load hosts"
          description={(hostsQuery.error as Error).message}
          action={
            <button type="button" onClick={() => hostsQuery.refetch()} className="infra-btn-primary">
              Retry
            </button>
          }
        />
      ) : (hostsQuery.data || []).length === 0 ? (
        <StateView
          kind="empty"
          title="No hosts enrolled"
          description="Generate a one-time enrollment token and start the InfraOS Go agent on an authorized macOS or Linux host to stream real telemetry."
          action={
            canManage && !session?.organization.is_demo ? (
              <button
                type="button"
                onClick={() => dialogRef.current?.showModal()}
                className="infra-btn-primary"
              >
                Enroll host agent
              </button>
            ) : undefined
          }
        />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Host</th>
                <th scope="col">Environment</th>
                <th scope="col">Status &amp; freshness</th>
                <th scope="col">CPU / Mem / Disk</th>
                <th scope="col">Last heartbeat</th>
                <th scope="col">Agent</th>
              </tr>
            </thead>
            <tbody>
              {(hostsQuery.data || []).map((h) => (
                <tr key={h.id}>
                  <th scope="row">
                    <Link
                      href={`/workspace/hosts/${h.id}`}
                      className="font-semibold text-[var(--text-primary)] hover:underline"
                    >
                      {h.display_name}
                    </Link>
                    <div className="font-mono text-xs font-normal text-[var(--text-secondary)]">
                      {h.hostname} · {h.ip_address}
                    </div>
                  </th>
                  <td data-label="Environment">{h.environment_name || "Unassigned"}</td>
                  <td data-label="Status">
                    <div className="flex flex-wrap gap-1.5">
                      <StatusBadge status={h.status} />
                      <StatusBadge status={h.freshness} />
                    </div>
                  </td>
                  <td data-label="Resources" className="font-mono text-xs">
                    {h.latest_metric
                      ? `${h.latest_metric.cpu_percent.toFixed(1)}% · ${h.latest_metric.mem_percent.toFixed(1)}% · ${h.latest_metric.disk_percent.toFixed(1)}%`
                      : "Awaiting sample"}
                  </td>
                  <td data-label="Last heartbeat" className="font-mono text-xs text-[var(--text-secondary)]">
                    {formatRelativeTime(h.last_heartbeat_at)}
                  </td>
                  <td data-label="Agent" className="font-mono text-xs">
                    {h.agent_version ? `v${h.agent_version}` : "None"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <dialog
        ref={dialogRef}
        aria-labelledby="enroll-dialog-title"
        onClick={(e) => {
          if (e.target === dialogRef.current) dialogRef.current?.close();
        }}
        className="infra-glass-overlay m-auto w-[min(94vw,560px)] rounded-2xl p-6 text-[var(--text-primary)]"
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <h2 id="enroll-dialog-title" className="text-lg font-bold">
              Enroll host agent
            </h2>
            <p className="mt-1 text-xs text-[var(--text-secondary)]">
              Issues a single-use token valid for 30 minutes. Raw credentials are shown once and stored hashed server-side.
            </p>
          </div>
          <button
            type="button"
            onClick={() => dialogRef.current?.close()}
            className="infra-btn-secondary min-h-[34px] px-3 text-xs"
          >
            Close
          </button>
        </div>

        {!tokenResult ? (
          <form onSubmit={handleGenerateToken} className="mt-5 space-y-4">
            <div className="space-y-1.5">
              <label htmlFor="enroll-label" className="block text-sm font-medium">
                Host label
              </label>
              <input
                id="enroll-label"
                value={enrollLabel}
                onChange={(e) => setEnrollLabel(e.target.value)}
                required
                className="infra-input"
              />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="enroll-env" className="block text-sm font-medium">
                Environment
              </label>
              <select
                id="enroll-env"
                value={enrollEnvId}
                onChange={(e) => setEnrollEnvId(e.target.value)}
                className="infra-input"
              >
                <option value="">Unassigned</option>
                {environments.map((env) => (
                  <option key={env.id} value={env.id}>
                    {env.name}
                  </option>
                ))}
              </select>
            </div>
            {enrollError ? (
              <p role="alert" className="text-xs text-[var(--status-err)]">
                {enrollError}
              </p>
            ) : null}
            <button type="submit" disabled={issuing} className="infra-btn-primary w-full">
              {issuing ? "Generating token..." : "Generate one-time enrollment token"}
            </button>
          </form>
        ) : (
          <div className="mt-5 space-y-4">
            <div className="rounded-xl border border-[var(--border)] bg-[var(--surface)] p-4 text-xs">
              <div className="font-semibold text-[var(--text-primary)]">
                Run on your authorized macOS or Linux host:
              </div>
              <pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all rounded-lg bg-[var(--surface-subtle)] p-3 font-mono text-[11px]">
{`INFRAOS_AGENT_ENROLL_TOKEN="${tokenResult.token}" \\
INFRAOS_AGENT_API_URL="${API_BASE}" \\
./bin/infraos-agent`}
              </pre>
              <p className="mt-2 text-[var(--text-secondary)]">
                Expires at <span className="font-mono">{tokenResult.expires_at}</span>. Single use only.
              </p>
            </div>
            <button
              type="button"
              onClick={() => {
                dialogRef.current?.close();
                hostsQuery.refetch();
              }}
              className="infra-btn-primary w-full"
            >
              Done
            </button>
          </div>
        )}
      </dialog>
    </div>
  );
}
