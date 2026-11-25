"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

interface ProjectItem {
  id: string;
  name: string;
  slug: string;
  description: string;
  repository_url: string;
}

export default function EnvironmentsPage() {
  const queryClient = useQueryClient();
  const { session, environments, canManage } = useWorkspace();
  const [envName, setEnvName] = useState("");
  const [tier, setTier] = useState("staging");
  const [desc, setDesc] = useState("");
  const [errMsg, setErrMsg] = useState("");

  const projectsQuery = useQuery({
    queryKey: ["projects", session?.organization.id],
    queryFn: () => apiFetch<ProjectItem[]>("/api/v1/projects"),
    enabled: Boolean(session),
  });

  async function handleCreateEnv(e: React.FormEvent) {
    e.preventDefault();
    setErrMsg("");
    try {
      await apiFetch("/api/v1/environments", {
        method: "POST",
        json: { name: envName, tier, description: desc },
      });
      setEnvName("");
      setDesc("");
      await queryClient.invalidateQueries({ queryKey: ["environments"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Could not create environment.");
    }
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-bold tracking-tight">Projects &amp; environments</h1>
        <p className="mt-1 text-sm text-[var(--text-secondary)]">
          Organize hosts, service monitors, deployment targets, and alert policies across logical tiers.
        </p>
      </header>

      <section className="grid gap-4 md:grid-cols-2">
        {(projectsQuery.data || []).map((p) => (
          <div key={p.id} className="infra-surface space-y-1.5 p-5">
            <div className="font-mono text-xs text-[var(--accent)]">{p.slug}</div>
            <h2 className="text-base font-semibold">{p.name}</h2>
            <p className="text-sm text-[var(--text-secondary)]">{p.description || "Infrastructure project."}</p>
          </div>
        ))}
      </section>

      {canManage ? (
        <form onSubmit={handleCreateEnv} className="infra-surface grid gap-4 p-5 md:grid-cols-[1fr_160px_1.2fr_auto] md:items-end">
          <div className="space-y-1.5">
            <label htmlFor="env-name" className="block text-xs font-semibold text-[var(--text-secondary)]">Environment name</label>
            <input id="env-name" value={envName} onChange={(e) => setEnvName(e.target.value)} required placeholder="QA Cluster" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="env-tier" className="block text-xs font-semibold text-[var(--text-secondary)]">Tier</label>
            <select id="env-tier" value={tier} onChange={(e) => setTier(e.target.value)} className="infra-input">
              <option value="development">Development</option>
              <option value="staging">Staging</option>
              <option value="production">Production</option>
            </select>
          </div>
          <div className="space-y-1.5">
            <label htmlFor="env-desc" className="block text-xs font-semibold text-[var(--text-secondary)]">Description</label>
            <input id="env-desc" value={desc} onChange={(e) => setDesc(e.target.value)} placeholder="Purpose of environment" className="infra-input" />
          </div>
          <button type="submit" className="infra-btn-primary">Add environment</button>
          {errMsg ? <p role="alert" className="md:col-span-4 text-xs text-[var(--status-err)]">{errMsg}</p> : null}
        </form>
      ) : null}

      {environments.length === 0 ? (
        <StateView kind="empty" title="No environments configured" description="Create an environment above to segment hosts and deployments." />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Environment</th>
                <th scope="col">Project</th>
                <th scope="col">Tier</th>
                <th scope="col">Description</th>
              </tr>
            </thead>
            <tbody>
              {environments.map((env) => (
                <tr key={env.id}>
                  <th scope="row" className="font-semibold">{env.name}</th>
                  <td data-label="Project">{env.project_name}</td>
                  <td data-label="Tier"><StatusBadge status={env.tier === "production" ? "warning" : "info"} label={env.tier} /></td>
                  <td data-label="Description" className="text-xs text-[var(--text-secondary)]">{env.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
