"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { AlertEvent, AlertRule } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";

export default function AlertsPage() {
  const queryClient = useQueryClient();
  const { session, canOperate } = useWorkspace();
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState("");
  const [ruleType, setRuleType] = useState("high_cpu");
  const [threshold, setThreshold] = useState(85);
  const [durationSeconds, setDurationSeconds] = useState(60);
  const [cooldownSeconds, setCooldownSeconds] = useState(300);
  const [severity, setSeverity] = useState("warning");
  const [autoIncident, setAutoIncident] = useState(false);
  const [errMsg, setErrMsg] = useState("");

  const rulesQuery = useQuery({
    queryKey: ["alerts", "rules", session?.organization.id],
    queryFn: () => apiFetch<AlertRule[]>("/api/v1/alerts/rules"),
    enabled: Boolean(session),
  });

  const eventsQuery = useQuery({
    queryKey: ["alerts", "events", session?.organization.id],
    queryFn: () => apiFetch<AlertEvent[]>("/api/v1/alerts/events"),
    enabled: Boolean(session),
  });

  async function handleCreateRule(e: React.FormEvent) {
    e.preventDefault();
    setErrMsg("");
    try {
      await apiFetch("/api/v1/alerts/rules", {
        method: "POST",
        json: {
          name,
          rule_type: ruleType,
          threshold: Number(threshold),
          duration_seconds: Number(durationSeconds),
          cooldown_seconds: Number(cooldownSeconds),
          severity,
          auto_incident: autoIncident,
        },
      });
      setName("");
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ["alerts"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Could not create alert rule.");
    }
  }

  async function handleDeleteRule(id: string) {
    await apiFetch(`/api/v1/alerts/rules/${id}`, { method: "DELETE" });
    await queryClient.invalidateQueries({ queryKey: ["alerts"] });
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Alert management</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Threshold rules with pending durations, cooldown anti-storm windows, and optional incident escalation.
          </p>
        </div>
        {canOperate ? (
          <button type="button" onClick={() => setShowForm((v) => !v)} className="infra-btn-primary">
            {showForm ? "Cancel" : "Create alert rule"}
          </button>
        ) : null}
      </header>

      {showForm ? (
        <form onSubmit={handleCreateRule} className="infra-surface grid gap-4 p-5 md:grid-cols-3">
          <div className="space-y-1.5 md:col-span-2">
            <label htmlFor="rule-name" className="block text-sm font-medium">Rule name</label>
            <input id="rule-name" value={name} onChange={(e) => setName(e.target.value)} required placeholder="Sustained high CPU" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="rule-type" className="block text-sm font-medium">Condition</label>
            <select id="rule-type" value={ruleType} onChange={(e) => setRuleType(e.target.value)} className="infra-input">
              <option value="high_cpu">High CPU (%)</option>
              <option value="high_memory">High Memory (%)</option>
              <option value="low_disk">High Disk Usage (%)</option>
              <option value="agent_offline">Agent Offline</option>
              <option value="service_down">Service Unavailable</option>
              <option value="high_latency">Excessive Latency (ms)</option>
              <option value="tls_expiring">TLS Expiry (days left)</option>
              <option value="deployment_failed">Deployment Failure</option>
            </select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="rule-threshold" className="block text-sm font-medium">Threshold</label>
            <input id="rule-threshold" type="number" value={threshold} onChange={(e) => setThreshold(Number(e.target.value))} className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="rule-dur" className="block text-sm font-medium">Pending duration (s)</label>
            <input id="rule-dur" type="number" min={0} value={durationSeconds} onChange={(e) => setDurationSeconds(Number(e.target.value))} className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="rule-cool" className="block text-sm font-medium">Cooldown (s)</label>
            <input id="rule-cool" type="number" min={30} value={cooldownSeconds} onChange={(e) => setCooldownSeconds(Number(e.target.value))} className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="rule-sev" className="block text-sm font-medium">Severity</label>
            <select id="rule-sev" value={severity} onChange={(e) => setSeverity(e.target.value)} className="infra-input">
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
            </select>
          </div>
          <div className="flex items-center gap-2 pt-6 md:col-span-2">
            <input id="rule-auto-inc" type="checkbox" checked={autoIncident} onChange={(e) => setAutoIncident(e.target.checked)} className="h-4 w-4" />
            <label htmlFor="rule-auto-inc" className="text-sm">Automatically open deduplicated incident when firing</label>
          </div>
          {errMsg ? <p role="alert" className="md:col-span-3 text-xs text-[var(--status-err)]">{errMsg}</p> : null}
          <div className="md:col-span-3">
            <button type="submit" className="infra-btn-primary">Save alert rule</button>
          </div>
        </form>
      ) : null}

      <section className="infra-surface overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-3.5">
          <h2 className="text-base font-semibold">Configured alert rules</h2>
        </div>
        {rulesQuery.isLoading ? (
          <div className="p-5 text-sm text-[var(--text-secondary)]">Loading alert rules...</div>
        ) : (rulesQuery.data || []).length === 0 ? (
          <div className="p-5 text-sm text-[var(--text-secondary)]">No alert rules configured yet.</div>
        ) : (
          <div className="infra-table-wrap">
            <table className="infra-table infra-table-stack">
              <thead>
                <tr>
                  <th scope="col">Rule</th>
                  <th scope="col">Condition</th>
                  <th scope="col">State</th>
                  <th scope="col">Severity</th>
                  <th scope="col">Cooldown</th>
                  <th scope="col">Actions</th>
                </tr>
              </thead>
              <tbody>
                {(rulesQuery.data || []).map((r) => (
                  <tr key={r.id}>
                    <th scope="row" className="font-semibold">
                      {r.name}
                      {r.auto_incident ? <span className="ml-2 font-mono text-[11px] font-normal text-[var(--accent)]">auto-incident</span> : null}
                    </th>
                    <td data-label="Condition" className="font-mono text-xs">{r.rule_type} ({r.threshold})</td>
                    <td data-label="State"><StatusBadge status={r.state} /></td>
                    <td data-label="Severity"><StatusBadge status={r.severity} /></td>
                    <td data-label="Cooldown" className="font-mono text-xs">{r.cooldown_seconds}s</td>
                    <td data-label="Actions">
                      {canOperate ? (
                        <button type="button" onClick={() => handleDeleteRule(r.id)} className="infra-btn-secondary min-h-[30px] px-2.5 text-xs">
                          Delete
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="infra-surface overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-3.5">
          <h2 className="text-base font-semibold">Alert history</h2>
        </div>
        {(eventsQuery.data || []).length === 0 ? (
          <div className="p-5 text-sm text-[var(--text-secondary)]">No alert events recorded yet.</div>
        ) : (
          <div className="divide-y divide-[var(--border)]">
            {(eventsQuery.data || []).map((ev) => (
              <div key={ev.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-3.5 text-sm">
                <div className="space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <StatusBadge status={ev.state} />
                    <StatusBadge status={ev.severity} />
                    <span className="font-semibold">{ev.rule_name || "Alert rule"}</span>
                  </div>
                  <p className="text-xs text-[var(--text-secondary)]">{ev.summary}</p>
                </div>
                <div className="flex items-center gap-3 font-mono text-xs text-[var(--text-secondary)]">
                  {ev.incident_id ? (
                    <Link href={`/workspace/incidents/${ev.incident_id}`} className="text-[var(--accent)] underline">
                      View incident
                    </Link>
                  ) : null}
                  <span>{formatRelativeTime(ev.triggered_at)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
