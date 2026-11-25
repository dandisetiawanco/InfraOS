"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { Role, SessionInfo } from "@infraos/contracts";
import { getClientApiBase, apiFetch } from "@/lib/api";

export interface EnvironmentOption {
  id: string;
  project_id: string;
  project_name: string;
  name: string;
  slug: string;
  tier: string;
  description: string;
}

interface WorkspaceContextValue {
  session: SessionInfo | null;
  isLoading: boolean;
  error: Error | null;
  environments: EnvironmentOption[];
  selectedEnvId: string;
  setSelectedEnvId: (id: string) => void;
  sseState: "connecting" | "live" | "polling";
  commandOpen: boolean;
  setCommandOpen: (open: boolean) => void;
  canOperate: boolean;
  canManage: boolean;
  refreshSession: () => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

function hasOperateRole(role?: Role): boolean {
  return role === "owner" || role === "admin" || role === "operator";
}

function hasManageRole(role?: Role): boolean {
  return role === "owner" || role === "admin";
}

export function WorkspaceProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const [selectedEnvId, setSelectedEnvId] = useState("");
  const [sseState, setSseState] = useState<"connecting" | "live" | "polling">("connecting");
  const [commandOpen, setCommandOpen] = useState(false);

  const sessionQuery = useQuery({
    queryKey: ["auth", "me"],
    queryFn: () => apiFetch<SessionInfo>("/api/v1/auth/me"),
    retry: false,
  });

  const envQuery = useQuery({
    queryKey: ["environments", sessionQuery.data?.organization.id],
    queryFn: () => apiFetch<EnvironmentOption[]>("/api/v1/environments"),
    enabled: Boolean(sessionQuery.data),
  });

  // Keyboard shortcut Cmd+K or Ctrl+K for command palette
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommandOpen((prev) => !prev);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  // Server-Sent Events subscription with automatic reconnect and polling fallback
  useEffect(() => {
    const orgId = sessionQuery.data?.organization.id;
    if (!orgId) return;

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let pollTimer: ReturnType<typeof setInterval> | null = null;
    let closed = false;
    let attempt = 0;

    const startPollingFallback = () => {
      setSseState("polling");
      if (!pollTimer) {
        pollTimer = setInterval(() => {
          queryClient.invalidateQueries({ queryKey: ["overview"] });
          queryClient.invalidateQueries({ queryKey: ["hosts"] });
          queryClient.invalidateQueries({ queryKey: ["services"] });
        }, 15_000);
      }
    };

    const stopPollingFallback = () => {
      if (pollTimer) {
        clearInterval(pollTimer);
        pollTimer = null;
      }
    };

    const connect = () => {
      if (closed) return;
      try {
        es = new EventSource(`${getClientApiBase()}/api/v1/events/stream`, { withCredentials: true });
      } catch {
        startPollingFallback();
        return;
      }

      es.addEventListener("connected", () => {
        attempt = 0;
        stopPollingFallback();
        setSseState("live");
      });

      const invalidate = (keys: string[][]) => () => {
        for (const key of keys) {
          queryClient.invalidateQueries({ queryKey: key });
        }
      };

      es.addEventListener("host.metrics", invalidate([["overview"], ["hosts"], ["host"]]));
      es.addEventListener("service.check", invalidate([["overview"], ["services"], ["service"]]));
      es.addEventListener("deployment.state", invalidate([["overview"], ["deployments"], ["deployment"]]));
      es.addEventListener("log.entry", invalidate([["logs"]]));
      es.addEventListener("alert.event", invalidate([["overview"], ["alerts"], ["notifications"]]));
      es.addEventListener("incident.updated", invalidate([["overview"], ["incidents"], ["incident"]]));

      es.onerror = () => {
        es?.close();
        es = null;
        startPollingFallback();
        attempt = Math.min(attempt + 1, 5);
        const delay = Math.min(30_000, 1500 * Math.pow(2, attempt));
        reconnectTimer = setTimeout(connect, delay);
      };
    };

    connect();

    return () => {
      closed = true;
      es?.close();
      if (reconnectTimer) clearTimeout(reconnectTimer);
      stopPollingFallback();
    };
  }, [sessionQuery.data?.organization.id, queryClient]);

  const refreshSession = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
  }, [queryClient]);

  const value = useMemo<WorkspaceContextValue>(
    () => ({
      session: sessionQuery.data ?? null,
      isLoading: sessionQuery.isLoading,
      error: (sessionQuery.error as Error) ?? null,
      environments: envQuery.data ?? [],
      selectedEnvId,
      setSelectedEnvId,
      sseState,
      commandOpen,
      setCommandOpen,
      canOperate: hasOperateRole(sessionQuery.data?.organization.role),
      canManage: hasManageRole(sessionQuery.data?.organization.role),
      refreshSession,
    }),
    [
      sessionQuery.data,
      sessionQuery.isLoading,
      sessionQuery.error,
      envQuery.data,
      selectedEnvId,
      sseState,
      commandOpen,
      refreshSession,
    ],
  );

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}

export function useWorkspace() {
  const ctx = useContext(WorkspaceContext);
  if (!ctx) {
    throw new Error("useWorkspace must be used inside WorkspaceProvider");
  }
  return ctx;
}
