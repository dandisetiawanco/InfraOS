"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { ThemeToggle } from "@/components/theme-toggle";
import { StatusBadge } from "@/components/status-badge";
import { formatBytes } from "@/lib/format";

interface BackupRecord {
  id: string;
  driver: string;
  file_path: string;
  size_bytes: number;
  sha256: string;
  status: string;
  notes: string;
  created_at: string;
}

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const { session, canManage } = useWorkspace();
  const [backupMsg, setBackupMsg] = useState("");
  const [backupErr, setBackupErr] = useState("");
  const [creating, setCreating] = useState(false);

  const backupsQuery = useQuery({
    queryKey: ["backups", session?.organization.id],
    queryFn: () => apiFetch<BackupRecord[]>("/api/v1/backups"),
    enabled: Boolean(session),
  });

  async function handleCreateBackup() {
    setBackupMsg("");
    setBackupErr("");
    setCreating(true);
    try {
      const b = await apiFetch<BackupRecord>("/api/v1/backups", { method: "POST" });
      setBackupMsg(`Consistent SQLite backup created (${b.file_path}, SHA-256 ${b.sha256.slice(0, 12)}...).`);
      await queryClient.invalidateQueries({ queryKey: ["backups"] });
    } catch (err) {
      setBackupErr(err instanceof APIError ? err.message : "Backup creation failed.");
    } finally {
      setCreating(false);
    }
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-bold tracking-tight">Workspace settings</h1>
        <p className="mt-1 text-sm text-[var(--text-secondary)]">
          Appearance preferences, active runtime profile, and database-consistent backup snapshots.
        </p>
      </header>

      <section className="infra-surface space-y-3 p-6">
        <h2 className="text-base font-semibold">Color appearance</h2>
        <p className="text-sm text-[var(--text-secondary)]">
          Choose Light, Dark, or follow your operating system scheme automatically.
        </p>
        <ThemeToggle variant="three-state" />
      </section>

      <section className="infra-surface grid gap-4 p-6 sm:grid-cols-3">
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Active profile</div>
          <div className="mt-1 font-mono text-sm font-semibold">{session?.profile || "lightweight"}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Database engine</div>
          <div className="mt-1 font-mono text-sm font-semibold">{session?.db_driver || "sqlite"}</div>
        </div>
        <div>
          <div className="text-xs text-[var(--text-secondary)]">Current organization role</div>
          <div className="mt-1 font-mono text-sm font-semibold capitalize">{session?.organization.role}</div>
        </div>
      </section>

      <section className="infra-surface space-y-4 p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold">Database backups &amp; integrity verification</h2>
            <p className="text-xs text-[var(--text-secondary)]">
              SQLite snapshots use transactional VACUUM INTO with SHA-256 checksums. PostgreSQL deployments use pg_dump scripts.
            </p>
          </div>
          {canManage ? (
            <button
              type="button"
              disabled={creating || session?.organization.is_demo}
              onClick={handleCreateBackup}
              className="infra-btn-primary"
            >
              {session?.organization.is_demo
                ? "Disabled in demo"
                : creating
                  ? "Creating snapshot..."
                  : "Create SQLite backup"}
            </button>
          ) : null}
        </div>

        {backupMsg ? <p role="status" className="text-xs text-[var(--status-ok)]">{backupMsg}</p> : null}
        {backupErr ? <p role="alert" className="text-xs text-[var(--status-err)]">{backupErr}</p> : null}

        {(backupsQuery.data || []).length === 0 ? (
          <p className="text-sm text-[var(--text-secondary)]">No backup snapshots recorded yet.</p>
        ) : (
          <div className="infra-table-wrap">
            <table className="infra-table infra-table-stack">
              <thead>
                <tr>
                  <th scope="col">Snapshot file</th>
                  <th scope="col">Driver</th>
                  <th scope="col">Size</th>
                  <th scope="col">SHA-256</th>
                  <th scope="col">Status</th>
                </tr>
              </thead>
              <tbody>
                {(backupsQuery.data || []).map((b) => (
                  <tr key={b.id}>
                    <th scope="row" className="font-mono text-xs">{b.file_path}</th>
                    <td data-label="Driver" className="font-mono text-xs">{b.driver}</td>
                    <td data-label="Size" className="font-mono text-xs">{formatBytes(b.size_bytes)}</td>
                    <td data-label="SHA-256" className="font-mono text-xs">{b.sha256.slice(0, 16)}…</td>
                    <td data-label="Status"><StatusBadge status={b.status} /></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
