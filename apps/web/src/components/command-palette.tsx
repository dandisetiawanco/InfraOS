"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  Bell,
  Box,
  FileText,
  GitBranch,
  HardDrive,
  Layers,
  LayoutDashboard,
  Network,
  Search,
  Settings,
  ShieldAlert,
  Users,
  X,
} from "lucide-react";
import { apiFetch } from "@/lib/api";
import { useWorkspace } from "./workspace-context";

interface SearchResult {
  type: string;
  id: string;
  title: string;
  subtitle: string;
  href: string;
}

const staticCommands = [
  { id: "nav-overview", title: "Open Overview", subtitle: "Workspace summary & telemetry", href: "/workspace", icon: LayoutDashboard },
  { id: "nav-hosts", title: "Navigate to Hosts", subtitle: "Inspect hosts & agent heartbeats", href: "/workspace/hosts", icon: HardDrive },
  { id: "nav-services", title: "Search Services", subtitle: "HTTP, HTTPS, TCP, DNS, and TLS probes", href: "/workspace/services", icon: Activity },
  { id: "nav-containers", title: "Inspect Containers", subtitle: "Docker integration & container runtime", href: "/workspace/containers", icon: Box },
  { id: "nav-deployments", title: "Find Deployments", subtitle: "Release history, locks, and rollbacks", href: "/workspace/deployments", icon: GitBranch },
  { id: "nav-logs", title: "Open Logs", subtitle: "Search structured logs & correlation IDs", href: "/workspace/logs", icon: FileText },
  { id: "nav-alerts", title: "View Alert Rules", subtitle: "Thresholds, cooldowns, and alert history", href: "/workspace/alerts", icon: Bell },
  { id: "nav-incidents", title: "View Incidents", subtitle: "Open incidents, timelines, and postmortems", href: "/workspace/incidents", icon: ShieldAlert },
  { id: "nav-topology", title: "Inspect Topology", subtitle: "Configured infrastructure relationship map", href: "/workspace/topology", icon: Network },
  { id: "nav-envs", title: "Manage Environments", subtitle: "Projects and deployment tiers", href: "/workspace/environments", icon: Layers },
  { id: "nav-team", title: "Team & Roles", subtitle: "Organization members and RBAC", href: "/workspace/team", icon: Users },
  { id: "nav-settings", title: "Open Settings", subtitle: "Theme, backups, and system profile", href: "/workspace/settings", icon: Settings },
];

export function CommandPalette() {
  const router = useRouter();
  const { commandOpen, setCommandOpen, environments, setSelectedEnvId } = useWorkspace();
  const [query, setQuery] = useState("");
  const dialogRef = useRef<HTMLDialogElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (commandOpen && !dialog.open) {
      dialog.showModal();
      setTimeout(() => inputRef.current?.focus(), 20);
    } else if (!commandOpen && dialog.open) {
      dialog.close();
      setQuery("");
    }
  }, [commandOpen]);

  const searchQuery = useQuery({
    queryKey: ["global-search", query],
    queryFn: () => apiFetch<SearchResult[]>(`/api/v1/search?q=${encodeURIComponent(query)}`),
    enabled: commandOpen && query.trim().length >= 2,
  });

  const filteredNav = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return staticCommands;
    return staticCommands.filter(
      (c) => c.title.toLowerCase().includes(q) || c.subtitle.toLowerCase().includes(q),
    );
  }, [query]);

  const runHref = (href: string) => {
    setCommandOpen(false);
    router.push(href);
  };

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby="command-palette-title"
      onClose={() => setCommandOpen(false)}
      onClick={(e) => {
        if (e.target === dialogRef.current) setCommandOpen(false);
      }}
      className="infra-glass-overlay m-auto w-[min(94vw,620px)] rounded-2xl p-0 text-[var(--text-primary)]"
    >
      <div className="flex items-center gap-3 border-b border-[var(--border)] px-4 py-3">
        <Search className="h-4 w-4 shrink-0 text-[var(--text-secondary)]" aria-hidden="true" />
        <label htmlFor="command-palette-input" id="command-palette-title" className="visually-hidden">
          Command palette search
        </label>
        <input
          ref={inputRef}
          id="command-palette-input"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Jump to hosts, services, deployments, incidents, or switch environment..."
          className="w-full bg-transparent text-sm text-[var(--text-primary)] outline-none placeholder:text-[var(--text-secondary)]"
        />
        <button
          type="button"
          onClick={() => setCommandOpen(false)}
          aria-label="Close command palette"
          className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)]"
        >
          <X className="h-4 w-4" aria-hidden="true" />
        </button>
      </div>

      <div className="max-h-[60vh] space-y-4 overflow-y-auto p-3">
        {searchQuery.data && searchQuery.data.length > 0 ? (
          <div>
            <div className="px-2 pb-1.5 text-xs font-semibold text-[var(--text-secondary)]">
              Matching resources
            </div>
            <ul className="space-y-1">
              {searchQuery.data.map((item) => (
                <li key={`${item.type}-${item.id}`}>
                  <button
                    type="button"
                    onClick={() => runHref(item.href)}
                    className="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm hover:bg-[var(--surface-subtle)]"
                  >
                    <span className="font-medium">{item.title}</span>
                    <span className="font-mono text-xs text-[var(--text-secondary)]">
                      {item.type} · {item.subtitle}
                    </span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        {environments.length > 0 ? (
          <div>
            <div className="px-2 pb-1.5 text-xs font-semibold text-[var(--text-secondary)]">
              Filter by environment
            </div>
            <div className="flex flex-wrap gap-1.5 px-2">
              <button
                type="button"
                onClick={() => {
                  setSelectedEnvId("");
                  setCommandOpen(false);
                }}
                className="infra-btn-secondary min-h-[34px] px-3 py-1 text-xs"
              >
                All environments
              </button>
              {environments.map((env) => (
                <button
                  key={env.id}
                  type="button"
                  onClick={() => {
                    setSelectedEnvId(env.id);
                    setCommandOpen(false);
                  }}
                  className="infra-btn-secondary min-h-[34px] px-3 py-1 text-xs"
                >
                  {env.name}
                </button>
              ))}
            </div>
          </div>
        ) : null}

        <div>
          <div className="px-2 pb-1.5 text-xs font-semibold text-[var(--text-secondary)]">
            Workspace commands
          </div>
          <ul className="space-y-1">
            {filteredNav.map(({ id, title, subtitle, href, icon: Icon }) => (
              <li key={id}>
                <button
                  type="button"
                  onClick={() => runHref(href)}
                  className="flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left hover:bg-[var(--surface-subtle)]"
                >
                  <Icon className="h-4 w-4 shrink-0 text-[var(--accent)]" aria-hidden="true" />
                  <span className="flex-1">
                    <span className="block text-sm font-medium">{title}</span>
                    <span className="block text-xs text-[var(--text-secondary)]">{subtitle}</span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <div className="flex items-center justify-between border-t border-[var(--border)] px-4 py-2 text-xs text-[var(--text-secondary)]">
        <span>Navigate with Tab and Enter</span>
        <span className="font-mono">Esc to close</span>
      </div>
    </dialog>
  );
}
