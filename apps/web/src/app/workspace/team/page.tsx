"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Role } from "@infraos/contracts";
import { apiFetch, APIError } from "@/lib/api";
import { useWorkspace } from "@/components/workspace-context";
import { StateView, StatusBadge } from "@/components/status-badge";

interface Member {
  user_id: string;
  email: string;
  name: string;
  role: Role;
  created_at: string;
}

export default function TeamPage() {
  const queryClient = useQueryClient();
  const { session, canManage } = useWorkspace();
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("viewer");
  const [errMsg, setErrMsg] = useState("");
  const [okMsg, setOkMsg] = useState("");

  const teamQuery = useQuery({
    queryKey: ["team", session?.organization.id],
    queryFn: () => apiFetch<Member[]>("/api/v1/team"),
    enabled: Boolean(session),
  });

  async function handleAddMember(e: React.FormEvent) {
    e.preventDefault();
    setErrMsg("");
    setOkMsg("");
    try {
      await apiFetch("/api/v1/team", {
        method: "POST",
        json: { email, name, password, role },
      });
      setEmail("");
      setName("");
      setPassword("");
      setOkMsg("Member added with server-enforced RBAC role.");
      await queryClient.invalidateQueries({ queryKey: ["team"] });
    } catch (err) {
      setErrMsg(err instanceof APIError ? err.message : "Could not add member.");
    }
  }

  async function handleChangeRole(userId: string, nextRole: Role) {
    await apiFetch(`/api/v1/team/${userId}`, { method: "PATCH", json: { role: nextRole } });
    await queryClient.invalidateQueries({ queryKey: ["team"] });
  }

  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-bold tracking-tight">Team &amp; access control</h1>
        <p className="mt-1 text-sm text-[var(--text-secondary)]">
          Server-side RBAC enforces Owner, Administrator, Operator, and read-only Viewer permissions.
        </p>
      </header>

      {canManage ? (
        <form onSubmit={handleAddMember} className="infra-surface grid gap-4 p-5 md:grid-cols-4 md:items-end">
          <div className="space-y-1.5">
            <label htmlFor="tm-name" className="block text-xs font-medium">Name</label>
            <input id="tm-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="Member Name" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="tm-email" className="block text-xs font-medium">Email</label>
            <input id="tm-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="engineer@example.com" className="infra-input" />
          </div>
          <div className="space-y-1.5">
            <label htmlFor="tm-pass" className="block text-xs font-medium">Initial password</label>
            <input id="tm-pass" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="Min 8 chars" className="infra-input" />
          </div>
          <div className="flex gap-2">
            <select aria-label="Role" value={role} onChange={(e) => setRole(e.target.value as Role)} className="infra-input">
              <option value="viewer">Viewer</option>
              <option value="operator">Operator</option>
              <option value="admin">Admin</option>
              <option value="owner">Owner</option>
            </select>
            <button type="submit" className="infra-btn-primary shrink-0">Add</button>
          </div>
          {okMsg ? <p role="status" className="md:col-span-4 text-xs text-[var(--status-ok)]">{okMsg}</p> : null}
          {errMsg ? <p role="alert" className="md:col-span-4 text-xs text-[var(--status-err)]">{errMsg}</p> : null}
        </form>
      ) : null}

      {teamQuery.isLoading ? (
        <StateView kind="loading" title="Loading members" description="Fetching organization roster and role assignments." />
      ) : (
        <div className="infra-surface infra-table-wrap">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Member</th>
                <th scope="col">Email</th>
                <th scope="col">Role</th>
                <th scope="col">Joined</th>
              </tr>
            </thead>
            <tbody>
              {(teamQuery.data || []).map((m) => (
                <tr key={m.user_id}>
                  <th scope="row" className="font-semibold">{m.name}</th>
                  <td data-label="Email" className="font-mono text-xs">{m.email}</td>
                  <td data-label="Role">
                    {canManage && m.user_id !== session?.user.id ? (
                      <select
                        aria-label={`Change role for ${m.email}`}
                        value={m.role}
                        onChange={(e) => handleChangeRole(m.user_id, e.target.value as Role)}
                        className="infra-input w-auto py-1 text-xs"
                      >
                        <option value="viewer">Viewer</option>
                        <option value="operator">Operator</option>
                        <option value="admin">Admin</option>
                        <option value="owner">Owner</option>
                      </select>
                    ) : (
                      <StatusBadge status="info" label={m.role} />
                    )}
                  </td>
                  <td data-label="Joined" className="font-mono text-xs text-[var(--text-secondary)]">
                    {m.created_at.slice(0, 10)}
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
