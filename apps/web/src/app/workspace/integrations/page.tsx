"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ContainerIntegrationStatus } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

interface IntegrationSetting {
  provider: string;
  status: string;
  config: Record<string, unknown>;
  secret_masked?: string;
}

interface GitHubOverview {
  connected: boolean;
  repository?: string;
  message: string;
  commits: Array<{ sha: string; message: string; author: string; date: string; url: string }>;
  workflow_runs: Array<{ id: number; name: string; status: string; conclusion: string; branch: string; commit_sha: string; html_url: string }>;
}

export default function IntegrationsPage() {
  const queryClient = useQueryClient();
  const { session, canManage } = useWorkspace();
  const [repo, setRepo] = useState("");
  const [ghToken, setGhToken] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [msg, setMsg] = useState("");
  const [errMsg, setErrMsg] = useState("");

  const integQuery = useQuery({
    queryKey: ["integrations", session?.organization.id],
    queryFn: () =>
      apiFetch<{
        settings: IntegrationSetting[];
        docker: ContainerIntegrationStatus;
        ai_enabled: boolean;
        otel: { enabled: boolean; endpoint: string };
      }>("/api/v1/integrations"),
    enabled: Boolean(session),
  });

  const ghQuery = useQuery({
    queryKey: ["integrations", "github", session?.organization.id],
    queryFn: () => apiFetch<GitHubOverview>("/api/v1/integrations/github/overview"),
    enabled: Boolean(session),
  });

  async function handleSaveGitHub(e: React.FormEvent) {
    e.preventDefault();
    setMsg("");
    setErrMsg("");
    try {
      await apiFetch("/api/v1/integrations/github", {
        method: "PUT",
        json: {
          config: { repository: repo, webhook_secret: webhookSecret },
          secret: ghToken,
        },
      });
      setGhToken("");
      setMsg("GitHub repository integration updated. Token encrypted server-side.");
      await queryClient.invalidateQueries({ queryKey: ["integrations"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Failed to update GitHub integration.");
    }
  }

  if (integQuery.isLoading) {
    return <StateView kind="loading" title="Checking integrations" description="Loading GitHub, Docker, Prometheus, OpenTelemetry, and AI provider status." />;
  }

  const gh = ghQuery.data;
  const docker = integQuery.data?.docker;

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-bold tracking-tight">Integrations</h1>
        <p className="mt-1 text-sm text-[var(--text-secondary)]">
          External connections report exact runtime verification state. Disconnected integrations never display fabricated data.
        </p>
      </header>

      <div className="grid gap-4 md:grid-cols-3">
        <div className="infra-surface space-y-2 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">Docker Engine</h2>
            <StatusBadge status={docker?.available ? "connected" : "offline"} label={docker?.mode || "disabled"} />
          </div>
          <p className="text-xs text-[var(--text-secondary)]">{docker?.message}</p>
        </div>

        <div className="infra-surface space-y-2 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">Prometheus &amp; OpenTelemetry</h2>
            <StatusBadge status="ok" label="/metrics active" />
          </div>
          <p className="text-xs text-[var(--text-secondary)]">
            Prometheus metrics are exposed on <code className="font-mono">/metrics</code>. External OTel export is{" "}
            {integQuery.data?.otel.enabled ? `enabled (${integQuery.data.otel.endpoint})` : "disabled by default"}.
          </p>
        </div>

        <div className="infra-surface space-y-2 p-5">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">Optional AI Summarizer</h2>
            <StatusBadge status={integQuery.data?.ai_enabled ? "connected" : "offline"} label={integQuery.data?.ai_enabled ? "Configured" : "Disabled"} />
          </div>
          <p className="text-xs text-[var(--text-secondary)]">
            External AI is disabled by default. Deterministic operational insights operate fully without API keys.
          </p>
        </div>
      </div>

      {/* GitHub integration */}
      <section className="infra-surface space-y-5 p-6">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h2 className="text-base font-semibold">GitHub repository &amp; signed webhooks</h2>
            <p className="text-xs text-[var(--text-secondary)]">{gh?.message}</p>
          </div>
          <StatusBadge status={gh?.connected ? "connected" : "offline"} label={gh?.connected ? "Connected" : "Disconnected"} />
        </div>

        {canManage && !session?.organization.is_demo ? (
          <form onSubmit={handleSaveGitHub} className="grid gap-4 md:grid-cols-3">
            <div className="space-y-1.5">
              <label htmlFor="gh-repo" className="block text-xs font-medium">Repository (owner/name)</label>
              <input id="gh-repo" value={repo} onChange={(e) => setRepo(e.target.value)} placeholder="dandisetiawanco/InfraOS" className="infra-input font-mono" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="gh-pat" className="block text-xs font-medium">Read-only token (optional for public repos)</label>
              <input id="gh-pat" type="password" value={ghToken} onChange={(e) => setGhToken(e.target.value)} placeholder="Stored AES-GCM encrypted server-side" className="infra-input" />
            </div>
            <div className="space-y-1.5">
              <label htmlFor="gh-hook" className="block text-xs font-medium">Webhook HMAC-SHA256 secret</label>
              <input id="gh-hook" type="password" value={webhookSecret} onChange={(e) => setWebhookSecret(e.target.value)} placeholder="Used to verify X-Hub-Signature-256" className="infra-input" />
            </div>
            {msg ? <p role="status" className="md:col-span-3 text-xs text-[var(--status-ok)]">{msg}</p> : null}
            {errMsg ? <p role="alert" className="md:col-span-3 text-xs text-[var(--status-err)]">{errMsg}</p> : null}
            <div className="md:col-span-3">
              <button type="submit" className="infra-btn-primary">Save GitHub configuration</button>
            </div>
          </form>
        ) : null}

        {gh?.connected && (gh.commits.length > 0 || gh.workflow_runs.length > 0) ? (
          <div className="grid gap-4 border-t border-[var(--border)] pt-4 lg:grid-cols-2">
            <div className="space-y-2">
              <h3 className="text-sm font-semibold">Recent commits ({gh.repository})</h3>
              <ul className="space-y-2 text-xs">
                {gh.commits.map((c) => (
                  <li key={c.sha} className="infra-surface-subtle p-3">
                    <div className="font-mono font-semibold text-[var(--accent)]">{c.sha}</div>
                    <div className="mt-0.5 text-[var(--text-primary)]">{c.message}</div>
                    <div className="mt-0.5 text-[var(--text-secondary)]">{c.author}</div>
                  </li>
                ))}
              </ul>
            </div>
            <div className="space-y-2">
              <h3 className="text-sm font-semibold">Workflow runs</h3>
              <ul className="space-y-2 text-xs">
                {gh.workflow_runs.map((r) => (
                  <li key={r.id} className="infra-surface-subtle flex items-center justify-between gap-2 p-3">
                    <div>
                      <div className="font-semibold">{r.name}</div>
                      <div className="font-mono text-[var(--text-secondary)]">{r.branch} · {r.commit_sha}</div>
                    </div>
                    <StatusBadge status={r.conclusion === "success" ? "ok" : r.status} label={r.conclusion || r.status} />
                  </li>
                ))}
              </ul>
            </div>
          </div>
        ) : null}
      </section>
    </div>
  );
}
