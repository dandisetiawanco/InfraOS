"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Deployment, DeploymentTarget } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/format";

export default function DeploymentsPage() {
  const queryClient = useQueryClient();
  const { session, selectedEnvId, environments, canOperate } = useWorkspace();

  const [showTargetForm, setShowTargetForm] = useState(false);
  const [targetName, setTargetName] = useState("");
  const [adapter, setAdapter] = useState("simulation");
  const [healthUrl, setHealthUrl] = useState("");
  const [targetEnvId, setTargetEnvId] = useState("");

  const [deployTargetId, setDeployTargetId] = useState("");
  const [version, setVersion] = useState("v1.0.1");
  const [commitMsg, setCommitMsg] = useState("");
  const branch = "main";
  const [feedback, setFeedback] = useState("");
  const [errMsg, setErrMsg] = useState("");
  const [busy, setBusy] = useState(false);

  const targetsQuery = useQuery({
    queryKey: ["deployment-targets", session?.organization.id, selectedEnvId],
    queryFn: () =>
      apiFetch<DeploymentTarget[]>(
        `/api/v1/deployment-targets${selectedEnvId ? `?environment_id=${encodeURIComponent(selectedEnvId)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  const depsQuery = useQuery({
    queryKey: ["deployments", session?.organization.id, selectedEnvId],
    queryFn: () =>
      apiFetch<Deployment[]>(
        `/api/v1/deployments${selectedEnvId ? `?environment_id=${encodeURIComponent(selectedEnvId)}` : ""}`,
      ),
    enabled: Boolean(session),
  });

  async function handleCreateTarget(e: React.FormEvent) {
    e.preventDefault();
    setErrMsg("");
    setFeedback("");
    try {
      await apiFetch("/api/v1/deployment-targets", {
        method: "POST",
        json: {
          name: targetName,
          adapter: session?.organization.is_demo ? "simulation" : adapter,
          health_url: healthUrl,
          environment_id: targetEnvId,
        },
      });
      setTargetName("");
      setHealthUrl("");
      setShowTargetForm(false);
      setFeedback("Deployment target registered.");
      await queryClient.invalidateQueries({ queryKey: ["deployment-targets"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Could not create target.");
    }
  }

  async function handleTriggerDeploy(e: React.FormEvent) {
    e.preventDefault();
    const chosenTarget = deployTargetId || (targetsQuery.data?.[0]?.id ?? "");
    if (!chosenTarget) {
      setErrMsg("Register a deployment target first.");
      return;
    }
    setBusy(true);
    setErrMsg("");
    setFeedback("");
    try {
      const dep = await apiFetch<Deployment>("/api/v1/deployments", {
        method: "POST",
        json: {
          target_id: chosenTarget,
          version,
          commit_message: commitMsg || `Release ${version}`,
          branch,
          idempotency_key: `ui-${chosenTarget}-${version}-${Date.now()}`,
        },
      });
      setFeedback(`Deployment ${dep.version} queued (${dep.is_simulation ? "simulation adapter" : "live verification"}).`);
      setCommitMsg("");
      await queryClient.invalidateQueries({ queryKey: ["deployments"] });
      await queryClient.invalidateQueries({ queryKey: ["deployment-targets"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Deployment trigger failed.");
    } finally {
      setBusy(false);
    }
  }

  const targets = targetsQuery.data || [];
  const deployments = depsQuery.data || [];

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Deployments</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            Controlled release workflows with per-target locking, idempotency keys, health verification, and artifact rollback.
          </p>
        </div>
        {canOperate ? (
          <button
            type="button"
            onClick={() => setShowTargetForm((v) => !v)}
            className="infra-btn-secondary"
          >
            {showTargetForm ? "Cancel" : "Add deployment target"}
          </button>
        ) : null}
      </header>

      {feedback ? (
        <div role="status" className="rounded-xl bg-[var(--status-ok-bg)] px-4 py-2.5 text-sm text-[var(--status-ok)]">
          {feedback}
        </div>
      ) : null}
      {errMsg ? (
        <div role="alert" className="rounded-xl bg-[var(--status-err-bg)] px-4 py-2.5 text-sm text-[var(--status-err)]">
          {errMsg}
        </div>
      ) : null}

      {showTargetForm ? (
        <form onSubmit={handleCreateTarget} className="infra-surface grid gap-4 p-5 md:grid-cols-2">
          <div className="space-y-1.5">
            <label htmlFor="tgt-name" className="block text-sm font-medium">Target name</label>
            <input id="tgt-name" value={targetName} onChange={(e) => setTargetName(e.target.value)} required placeholder="Sample App Local" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="tgt-adapter" className="block text-sm font-medium">Adapter</label>
            <select
              id="tgt-adapter"
              value={session?.organization.is_demo ? "simulation" : adapter}
              onChange={(e) => setAdapter(e.target.value)}
              disabled={session?.organization.is_demo}
              className="infra-input"
            >
              <option value="simulation">Simulation (labeled safe dry-run)</option>
              <option value="local_http">Local HTTP health-verified target</option>
              <option value="docker_compose">Docker Compose target</option>
            </select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="tgt-health" className="block text-sm font-medium">Health verification URL (optional)</label>
            <input id="tgt-health" value={healthUrl} onChange={(e) => setHealthUrl(e.target.value)} placeholder="http://127.0.0.1:8090/healthz" className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="tgt-env" className="block text-sm font-medium">Environment</label>
            <select id="tgt-env" value={targetEnvId} onChange={(e) => setTargetEnvId(e.target.value)} className="infra-input">
              <option value="">Unassigned</option>
              {environments.map((env) => (
                <option key={env.id} value={env.id}>{env.name}</option>
              ))}
            </select>
          </div>
          <div className="md:col-span-2">
            <button type="submit" className="infra-btn-primary">Register target</button>
          </div>
        </form>
      ) : null}

      {/* Trigger deployment panel */}
      {canOperate && targets.length > 0 ? (
        <form onSubmit={handleTriggerDeploy} className="infra-surface grid gap-4 p-5 lg:grid-cols-[1.2fr_0.8fr_1.4fr_auto] lg:items-end">
          <div className="space-y-1.5">
            <label htmlFor="dep-target" className="block text-xs font-semibold text-[var(--text-secondary)]">
              Deployment target
            </label>
            <select
              id="dep-target"
              value={deployTargetId || targets[0]?.id || ""}
              onChange={(e) => setDeployTargetId(e.target.value)}
              className="infra-input"
            >
              {targets.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name} ({t.adapter}) {t.locked_by_deployment_id ? "[LOCKED]" : ""}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="dep-version" className="block text-xs font-semibold text-[var(--text-secondary)]">
              Release version
            </label>
            <input id="dep-version" value={version} onChange={(e) => setVersion(e.target.value)} required className="infra-input font-mono" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="dep-msg" className="block text-xs font-semibold text-[var(--text-secondary)]">
              Release note
            </label>
            <input id="dep-msg" value={commitMsg} onChange={(e) => setCommitMsg(e.target.value)} placeholder="Describe change being verified" className="infra-input" />
          </div>
          <button type="submit" disabled={busy} className="infra-btn-primary">
            {busy ? "Queuing..." : "Start deployment"}
          </button>
        </form>
      ) : null}

      {/* Targets summary */}
      {targets.length > 0 ? (
        <section className="grid gap-3 md:grid-cols-2">
          {targets.map((t) => (
            <div key={t.id} className="infra-surface flex items-center justify-between gap-4 p-4">
              <div>
                <div className="font-semibold">{t.name}</div>
                <div className="font-mono text-xs text-[var(--text-secondary)]">
                  adapter: {t.adapter} · active: {t.current_version || "none"}
                </div>
              </div>
              <div className="flex items-center gap-2">
                {t.adapter === "simulation" ? <StatusBadge status="simulation" label="Simulation" /> : null}
                {t.locked_by_deployment_id ? <StatusBadge status="pending" label="Locked" /> : <StatusBadge status="ok" label="Unlocked" />}
              </div>
            </div>
          ))}
        </section>
      ) : null}

      {/* History table */}
      {depsQuery.isLoading ? (
        <StateView kind="loading" title="Loading deployments" description="Retrieving release history and verification records." />
      ) : deployments.length === 0 ? (
        <StateView
          kind="empty"
          title="No deployments recorded"
          description="Register a deployment target above to run verified health-checked releases or clearly labeled simulation deployments."
        />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Release</th>
                <th scope="col">Target &amp; mode</th>
                <th scope="col">Commit</th>
                <th scope="col">Status</th>
                <th scope="col">Triggered</th>
                <th scope="col">Inspect</th>
              </tr>
            </thead>
            <tbody>
              {deployments.map((d) => (
                <tr key={d.id}>
                  <th scope="row">
                    <Link href={`/workspace/deployments/${d.id}`} className="font-mono font-semibold hover:underline">
                      {d.version}
                    </Link>
                    <div className="text-xs font-normal text-[var(--text-secondary)]">{d.commit_message || d.trigger_type}</div>
                  </th>
                  <td data-label="Target">
                    <div className="text-sm">{d.target_name}</div>
                    <div className="mt-0.5 flex gap-1.5">
                      {d.is_simulation ? <StatusBadge status="simulation" label="Simulation" /> : null}
                    </div>
                  </td>
                  <td data-label="Commit" className="font-mono text-xs">
                    {d.commit_sha.slice(0, 10)} ({d.branch})
                  </td>
                  <td data-label="Status"><StatusBadge status={d.status} /></td>
                  <td data-label="Triggered" className="font-mono text-xs text-[var(--text-secondary)]">
                    {formatRelativeTime(d.created_at)}
                  </td>
                  <td data-label="Inspect">
                    <Link href={`/workspace/deployments/${d.id}`} className="infra-btn-secondary min-h-[32px] px-2.5 text-xs">
                      Timeline
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
