"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  Bell,
  Box,
  FileText,
  GitBranch,
  HardDrive,
  History,
  Layers,
  LayoutDashboard,
  LogOut,
  Menu,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Plug,
  Search,
  Settings,
  ShieldAlert,
  Users,
  X,
} from "lucide-react";
import type { NotificationItem } from "@infraos/contracts";
import { apiFetch } from "@/lib/api";
import { BrandMark } from "./brand-mark";
import { ThemeToggle } from "./theme-toggle";
import { CommandPalette } from "./command-palette";
import { useWorkspace } from "./workspace-context";
import { formatRelativeTime } from "@/lib/format";

const navGroups = [
  {
    label: "Core",
    items: [
      { href: "/workspace", label: "Overview", icon: LayoutDashboard, exact: true },
      { href: "/workspace/hosts", label: "Hosts", icon: HardDrive },
      { href: "/workspace/services", label: "Services", icon: Activity },
      { href: "/workspace/containers", label: "Containers", icon: Box },
      { href: "/workspace/deployments", label: "Deployments", icon: GitBranch },
    ],
  },
  {
    label: "Observability",
    items: [
      { href: "/workspace/logs", label: "Logs", icon: FileText },
      { href: "/workspace/alerts", label: "Alerts", icon: Bell },
      { href: "/workspace/incidents", label: "Incidents", icon: ShieldAlert },
      { href: "/workspace/topology", label: "Topology", icon: Network },
    ],
  },
  {
    label: "Administration",
    items: [
      { href: "/workspace/environments", label: "Environments", icon: Layers },
      { href: "/workspace/integrations", label: "Integrations", icon: Plug },
      { href: "/workspace/audit", label: "Audit Trail", icon: History },
      { href: "/workspace/team", label: "Team", icon: Users },
      { href: "/workspace/settings", label: "Settings", icon: Settings },
    ],
  },
];

export function WorkspaceShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const {
    session,
    isLoading,
    environments,
    selectedEnvId,
    setSelectedEnvId,
    sseState,
    setCommandOpen,
    refreshSession,
  } = useWorkspace();

  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);
  const [notifOpen, setNotifOpen] = useState(false);
  const mobileDialogRef = useRef<HTMLDialogElement>(null);

  const notifQuery = useQuery({
    queryKey: ["notifications", session?.organization.id],
    queryFn: () => apiFetch<NotificationItem[]>("/api/v1/notifications"),
    enabled: Boolean(session),
  });

  useEffect(() => {
    const dialog = mobileDialogRef.current;
    if (!dialog) return;
    if (mobileOpen && !dialog.open) dialog.showModal();
    if (!mobileOpen && dialog.open) dialog.close();
  }, [mobileOpen]);

  useEffect(() => {
    setMobileOpen(false);
    setNotifOpen(false);
  }, [pathname]);

  if (isLoading) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-[var(--canvas)] p-6">
        <div className="infra-surface max-w-sm space-y-2 p-6 text-center">
          <div className="mx-auto flex justify-center">
            <BrandMark size={36} />
          </div>
          <p className="text-sm font-medium text-[var(--text-primary)]">
            Loading InfraOS workspace...
          </p>
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-[var(--canvas)] p-6">
        <div className="infra-surface max-w-md space-y-4 p-7">
          <div className="inline-flex items-center gap-2.5 text-base font-bold">
            <BrandMark size={30} />
            <span>InfraOS</span>
          </div>
          <h1 className="text-xl font-bold tracking-tight">Workspace authentication required</h1>
          <p className="text-sm text-[var(--text-secondary)]">
            Sign in to monitor authorized hosts and services, or start an isolated synthetic
            demonstration session.
          </p>
          <div className="flex flex-wrap gap-3 pt-1">
            <Link href="/login" className="infra-btn-primary">
              Sign in
            </Link>
            <Link href="/demo" className="infra-btn-secondary">
              Launch isolated demo
            </Link>
            <Link href="/" className="infra-btn-secondary">
              Back to home
            </Link>
          </div>
        </div>
      </div>
    );
  }

  const unreadCount = (notifQuery.data || []).filter((n) => !n.read_at).length;

  async function handleSwitchOrg(orgId: string) {
    await apiFetch("/api/v1/auth/switch-org", {
      method: "POST",
      json: { organization_id: orgId },
    });
    setSelectedEnvId("");
    await refreshSession();
    await queryClient.invalidateQueries();
  }

  async function handleLogout() {
    await apiFetch("/api/v1/auth/logout", { method: "POST" });
    queryClient.clear();
    router.push("/login");
  }

  async function handleMarkAllRead() {
    await apiFetch("/api/v1/notifications/read-all", { method: "POST" });
    await queryClient.invalidateQueries({ queryKey: ["notifications"] });
  }

  const isLinkActive = (href: string, exact?: boolean) =>
    exact ? pathname === href : pathname === href || pathname.startsWith(href + "/");

  const renderNavItems = (iconOnly: boolean) => (
    <nav aria-label="Workspace" className="flex-1 space-y-5 overflow-y-auto px-2.5 py-3">
      {navGroups.map((group) => (
        <div key={group.label} className="space-y-1">
          {!iconOnly ? (
            <div className="px-2.5 pb-1 text-xs font-semibold text-[var(--text-secondary)]">
              {group.label}
            </div>
          ) : null}
          <ul className="space-y-0.5">
            {group.items.map(({ href, label, icon: Icon, exact }) => {
              const active = isLinkActive(href, exact);
              return (
                <li key={href}>
                  <Link
                    href={href}
                    aria-current={active ? "page" : undefined}
                    aria-label={iconOnly ? label : undefined}
                    title={iconOnly ? label : undefined}
                    className={`flex min-h-[40px] items-center gap-3 rounded-xl px-2.5 text-sm font-medium transition-colors ${
                      active
                        ? "bg-[var(--accent-soft)] font-semibold text-[var(--text-primary)] ring-1 ring-inset ring-[var(--accent)]"
                        : "text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--text-primary)]"
                    } ${iconOnly ? "justify-center" : ""}`}
                  >
                    <Icon
                      className={`h-4 w-4 shrink-0 ${
                        active ? "text-[var(--accent)]" : "text-[var(--text-secondary)]"
                      }`}
                      aria-hidden="true"
                    />
                    {!iconOnly ? <span className="truncate">{label}</span> : null}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );

  return (
    <div className="flex min-h-dvh bg-[var(--canvas)] text-[var(--text-primary)]">
      {/* Desktop Floating Liquid Glass Sidebar */}
      <aside
        className={`sticky top-0 hidden h-dvh shrink-0 p-3 transition-[width] duration-200 lg:flex ${
          collapsed ? "w-[84px]" : "w-[260px]"
        }`}
      >
        <div className="infra-glass-nav flex h-full w-full flex-col overflow-hidden rounded-2xl">
          <div className="flex items-center justify-between gap-2 border-b border-[var(--border)] px-3.5 py-3">
            <Link href="/workspace" className="inline-flex min-w-0 items-center gap-2.5 font-bold">
              <BrandMark size={28} />
              {!collapsed ? (
                <span className="truncate text-base tracking-tight">
                  Infra<span className="text-[var(--accent)]">OS</span>
                </span>
              ) : null}
            </Link>
            <button
              type="button"
              onClick={() => setCollapsed((v) => !v)}
              aria-expanded={!collapsed}
              aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
              className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--text-primary)]"
            >
              {collapsed ? (
                <PanelLeftOpen className="h-4 w-4" aria-hidden="true" />
              ) : (
                <PanelLeftClose className="h-4 w-4" aria-hidden="true" />
              )}
            </button>
          </div>

          {renderNavItems(collapsed)}

          <div className="border-t border-[var(--border)] p-3">
            {!collapsed ? (
              <div className="space-y-2">
                <div className="flex items-center justify-between text-xs text-[var(--text-secondary)]">
                  <span>Stream</span>
                  <span className="font-mono text-[var(--text-primary)]">
                    {sseState === "live" ? "SSE live" : sseState === "polling" ? "Polling" : "Connecting"}
                  </span>
                </div>
                <div className="truncate text-xs text-[var(--text-secondary)]">
                  {session.user.email} · <span className="capitalize">{session.organization.role}</span>
                </div>
              </div>
            ) : (
              <div className="flex justify-center">
                <span
                  className="h-2.5 w-2.5 rounded-full"
                  style={{
                    backgroundColor:
                      sseState === "live" ? "var(--status-ok)" : "var(--status-warn)",
                  }}
                  title={`Stream: ${sseState}`}
                />
              </div>
            )}
          </div>
        </div>
      </aside>

      {/* Mobile Navigation Drawer (<dialog>) */}
      <dialog
        ref={mobileDialogRef}
        aria-label="Workspace navigation"
        onClose={() => setMobileOpen(false)}
        onClick={(e) => {
          if (e.target === mobileDialogRef.current) setMobileOpen(false);
        }}
        className="infra-glass-overlay m-0 h-dvh max-h-dvh w-[min(86vw,300px)] rounded-r-2xl p-0 text-[var(--text-primary)]"
      >
        <div className="flex h-full flex-col">
          <div className="flex items-center justify-between border-b border-[var(--border)] px-4 py-3.5">
            <span className="inline-flex items-center gap-2 font-bold">
              <BrandMark size={26} />
              InfraOS
            </span>
            <button
              type="button"
              onClick={() => setMobileOpen(false)}
              aria-label="Close navigation menu"
              className="infra-btn-secondary h-10 w-10 p-0"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          </div>
          {renderNavItems(false)}
          <div className="border-t border-[var(--border)] p-4">
            <button type="button" onClick={handleLogout} className="infra-btn-secondary w-full">
              <LogOut className="h-4 w-4" aria-hidden="true" />
              <span>Sign out</span>
            </button>
          </div>
        </div>
      </dialog>

      {/* Right Main Column */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Contextual Liquid Glass Toolbar */}
        <header className="sticky top-0 z-20 px-3 pt-3 sm:px-5">
          <div className="infra-glass-nav flex flex-wrap items-center justify-between gap-2 rounded-2xl px-3 py-2 sm:px-4">
            <div className="flex min-w-0 items-center gap-2">
              <button
                type="button"
                onClick={() => setMobileOpen(true)}
                aria-label="Open workspace menu"
                aria-expanded={mobileOpen}
                className="infra-btn-secondary inline-flex px-2.5 lg:hidden"
              >
                <Menu className="h-4 w-4" aria-hidden="true" />
                <span className="text-xs">Menu</span>
              </button>

              <label htmlFor="org-switcher" className="visually-hidden">
                Organization
              </label>
              <select
                id="org-switcher"
                value={session.organization.id}
                onChange={(e) => handleSwitchOrg(e.target.value)}
                className="infra-input max-w-[190px] py-1.5 text-xs font-medium sm:max-w-[230px]"
              >
                {session.organizations.map((org) => (
                  <option key={org.id} value={org.id}>
                    {org.name}
                  </option>
                ))}
              </select>

              <label htmlFor="env-switcher" className="visually-hidden">
                Environment filter
              </label>
              <select
                id="env-switcher"
                value={selectedEnvId}
                onChange={(e) => setSelectedEnvId(e.target.value)}
                className="infra-input hidden max-w-[165px] py-1.5 text-xs sm:block"
              >
                <option value="">All environments</option>
                {environments.map((env) => (
                  <option key={env.id} value={env.id}>
                    {env.name}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setCommandOpen(true)}
                aria-label="Open command palette"
                className="infra-btn-secondary min-h-[36px] gap-2 px-3 py-1.5 text-xs text-[var(--text-secondary)]"
              >
                <Search className="h-3.5 w-3.5" aria-hidden="true" />
                <span className="hidden sm:inline">Command palette</span>
                <kbd className="hidden rounded border border-[var(--border-strong)] px-1.5 py-0.5 font-mono text-[10px] md:inline">
                  ⌘K
                </kbd>
              </button>

              <div className="relative">
                <button
                  type="button"
                  onClick={() => setNotifOpen((v) => !v)}
                  aria-label="Notifications"
                  aria-expanded={notifOpen}
                  className="infra-touch-target relative inline-flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] hover:bg-[var(--surface-subtle)]"
                >
                  <Bell className="h-4 w-4" aria-hidden="true" />
                  {unreadCount > 0 ? (
                    <span className="absolute -right-1 -top-1 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-[var(--accent)] px-1 font-mono text-[10px] font-bold text-[var(--accent-contrast)]">
                      {unreadCount}
                    </span>
                  ) : null}
                </button>

                {notifOpen ? (
                  <div
                    role="region"
                    aria-label="Notification center"
                    className="infra-glass-overlay absolute right-0 top-11 z-40 w-[min(90vw,360px)] rounded-2xl p-4"
                  >
                    <div className="mb-3 flex items-center justify-between">
                      <h2 className="text-sm font-semibold">Notifications</h2>
                      {unreadCount > 0 ? (
                        <button
                          type="button"
                          onClick={handleMarkAllRead}
                          className="text-xs font-medium text-[var(--accent)] underline"
                        >
                          Mark all read
                        </button>
                      ) : null}
                    </div>
                    <div className="max-h-72 space-y-2 overflow-y-auto">
                      {(notifQuery.data || []).length === 0 ? (
                        <p className="py-4 text-xs text-[var(--text-secondary)]">
                          No notifications recorded yet.
                        </p>
                      ) : (
                        (notifQuery.data || []).map((n) => (
                          <Link
                            key={n.id}
                            href={n.link || "/workspace"}
                            onClick={() => setNotifOpen(false)}
                            className="block rounded-xl border border-[var(--border)] bg-[var(--surface)] p-3 hover:bg-[var(--surface-subtle)]"
                          >
                            <div className="flex items-center justify-between gap-2 text-xs font-semibold">
                              <span>{n.title}</span>
                              <span className="font-mono text-[11px] font-normal text-[var(--text-secondary)]">
                                {formatRelativeTime(n.created_at)}
                              </span>
                            </div>
                            <p className="mt-1 text-xs text-[var(--text-secondary)]">{n.body}</p>
                          </Link>
                        ))
                      )}
                    </div>
                  </div>
                ) : null}
              </div>

              <ThemeToggle variant="compact" />

              <button
                type="button"
                onClick={handleLogout}
                aria-label="Sign out"
                title="Sign out"
                className="infra-touch-target hidden h-9 w-9 items-center justify-center rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--text-primary)] sm:inline-flex"
              >
                <LogOut className="h-4 w-4" aria-hidden="true" />
              </button>
            </div>
          </div>
        </header>

        {session.organization.is_demo ? (
          <div
            role="status"
            className="mx-3 mt-3 rounded-xl border border-[var(--accent)] bg-[var(--accent-soft)] px-4 py-2.5 text-xs text-[var(--text-primary)] sm:mx-5"
          >
            <strong>Isolated synthetic demo mode:</strong> All hosts, telemetry history, containers,
            and incidents shown in this workspace are synthetic demonstration records. Real host
            sockets and external monitoring targets are disabled.
          </div>
        ) : null}

        <main id="main-content" className="min-w-0 flex-1 p-3 sm:p-5">
          {children}
        </main>
      </div>

      <CommandPalette />
    </div>
  );
}
