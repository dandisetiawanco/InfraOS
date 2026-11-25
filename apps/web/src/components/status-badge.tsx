export function StatusBadge({
  status,
  label,
}: {
  status: string;
  label?: string;
}) {
  const s = status.toLowerCase();
  let tone = {
    bg: "var(--surface-subtle)",
    fg: "var(--text-secondary)",
    dot: "var(--text-secondary)",
  };

  if (
    ["online", "healthy", "up", "running", "succeeded", "rolled_back", "connected", "verified", "live", "ok", "resolved", "active"].includes(s)
  ) {
    tone = { bg: "var(--status-ok-bg)", fg: "var(--status-ok)", dot: "var(--status-ok)" };
  } else if (
    ["stale", "degraded", "warning", "pending", "investigating", "identified", "monitoring", "queued", "preparing", "building", "deploying", "verifying", "rolling_back", "sev3"].includes(s)
  ) {
    tone = { bg: "var(--status-warn-bg)", fg: "var(--status-warn)", dot: "var(--status-warn)" };
  } else if (
    ["offline", "down", "failed", "critical", "firing", "revoked", "error", "unhealthy", "open", "sev1", "sev2", "fatal"].includes(s)
  ) {
    tone = { bg: "var(--status-err-bg)", fg: "var(--status-err)", dot: "var(--status-err)" };
  } else if (["synthetic", "simulation", "demo", "info", "sev4"].includes(s)) {
    tone = { bg: "var(--status-info-bg)", fg: "var(--status-info)", dot: "var(--status-info)" };
  }

  const displayText = label || status.replace(/_/g, " ");

  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium capitalize"
      style={{ backgroundColor: tone.bg, color: tone.fg }}
    >
      <span
        className="h-1.5 w-1.5 rounded-full"
        style={{ backgroundColor: tone.dot }}
        aria-hidden="true"
      />
      <span>{displayText}</span>
    </span>
  );
}

export function DataFreshnessTag({
  source,
  updatedAt,
}: {
  source: string;
  updatedAt?: string;
}) {
  const labels: Record<string, string> = {
    live: "Live host telemetry",
    stale: "Stale telemetry",
    unavailable: "No telemetry received yet",
    synthetic: "Synthetic demo data",
    historical: "Historical sample",
  };
  return (
    <div className="inline-flex flex-wrap items-center gap-2 text-xs text-[var(--text-secondary)]">
      <StatusBadge status={source} label={labels[source] || source} />
      {updatedAt ? <span className="font-mono">{updatedAt}</span> : null}
    </div>
  );
}

export function StateView({
  kind,
  title,
  description,
  action,
}: {
  kind: "loading" | "empty" | "error";
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div
      className="infra-surface flex flex-col items-start gap-3 p-6"
      role={kind === "error" ? "alert" : "status"}
    >
      <h2 className="text-base font-semibold text-[var(--text-primary)]">{title}</h2>
      <p className="max-w-xl text-sm text-[var(--text-secondary)]">{description}</p>
      {action ? <div className="pt-1">{action}</div> : null}
    </div>
  );
}
