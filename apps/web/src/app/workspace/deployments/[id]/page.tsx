"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Deployment } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

export default function DeploymentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const { session, canOperate } = useWorkspace();
  const [rbErr, setRbErr] = useState("");
  const [rollingBack, setRollingBack] = useState(false);

  const depQuery = useQuery({
    queryKey: ["deployment", id],
    queryFn: () => apiFetch<Deployment>(`/api/v1/deployments/${id}`),
    enabled: Boolean(session && id),
    refetchInterval: (q) => {
      const st = q.state.data?.status;
      return st && ["queued", "preparing", "building", "deploying", "verifying", "rolling_back"].includes(st)
        ? 2000
        : false;
    },
  });

  if (depQuery.isLoading) {
    return <StateView kind="loading" title="Loading deployment" description="Reading release metadata and state transitions." />;
  }
  if (depQuery.isError || !depQuery.data) {
    return (
      <StateView
        kind="error"
        title="Deployment not found"
        description={(depQuery.error as Error)?.message || "Could not load deployment."}
        action={<Link href="/workspace/deployments" className="infra-btn-secondary">Back to deployments</Link>}
      />
    );
  }

  const dep = depQuery.data;

  async function handleRollback() {
    setRbErr("");
    setRollingBack(true);
    try {
      const rb = await apiFetch<Deployment>(`/api/v1/deployments/${dep.id}/rollback`, {
        method: "POST",
      });
      await queryClient.invalidateQueries({ queryKey: ["deployments"] });
      router.push(`/workspace/deployments/${rb.id}`);
    } catch (err) {
      setRbErr(err instanceof APIError ? err.message : "Rollback failed.");
      setRollingBack(false);
    }
  }

  return (
    <div className="space-y-6">
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="space-y-1">
          <Link href="/workspace/deployments" className="text-xs text-[var(--text-secondary)] hover:underline">
            Deployments
          </Link>
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="text-2xl font-bold tracking-tight">
              {dep.target_name} · <span className="font-mono">{dep.version}</span>
            </h1>
            <StatusBadge status={dep.status} />
            {dep.is_simulation ? <StatusBadge status="simulation" label="Simulation mode" /> : null}
          </div>
          <p className="font-mono text-xs text-[var(--text-secondary)]">
            commit {dep.commit_sha} · branch {dep.branch} · trigger {dep.trigger_type}
          </p>
        </div>

        {canOperate ? (
          <button
            type="button"
            disabled={rollingBack}
            onClick={handleRollback}
            className="infra-btn-secondary"
          >
            {rollingBack ? "Initiating rollback..." : "Roll back to previous artifact"}
          </button>
        ) : null}
      </header>

      {rbErr ? (
        <div role="alert" className="rounded-xl bg-[var(--status-err-bg)] px-4 py-2.5 text-sm text-[var(--status-err)]">
          {rbErr}
        </div>
      ) : null}

      {dep.error_message ? (
        <div role="alert" className="infra-surface border-[var(--status-err)] p-4 text-sm text-[var(--status-err)]">
          <strong>Failure report:</strong> {dep.error_message}
        </div>
      ) : null}

      <section className="infra-surface overflow-hidden">
        <div className="border-b border-[var(--border)] px-5 py-3.5">
          <h2 className="text-base font-semibold">State transitions &amp; deployment output</h2>
        </div>
        <ol className="divide-y divide-[var(--border)]">
          {(dep.events || []).map((ev) => (
            <li key={ev.id} className="flex flex-wrap items-start justify-between gap-3 px-5 py-3.5 text-sm">
              <div className="space-y-1">
                <div className="flex items-center gap-2">
                  <StatusBadge status={ev.state} />
                  <span className="font-mono text-xs text-[var(--text-secondary)]">{ev.level}</span>
                </div>
                <p className="font-mono text-xs text-[var(--text-primary)]">{ev.message}</p>
              </div>
              <span className="font-mono text-xs text-[var(--text-secondary)]">{ev.created_at}</span>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
