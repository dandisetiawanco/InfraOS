import type { Metadata } from "next";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "Features",
  description: "Detailed breakdown of InfraOS monitoring, deployment, container, logging, and incident capabilities.",
};

const capabilities = [
  {
    area: "Monitoring & Telemetry",
    items: [
      "Native Go host agent supporting macOS and Linux without root privileges",
      "Real CPU, memory, load average, filesystem capacity, and counter-reset-safe network throughput",
      "Downsampled time-series history (15m, 1h, 24h, 7d) with Server-Sent Events live updates",
      "HTTP, HTTPS, TCP, DNS, and TLS certificate expiration monitoring scheduler",
    ],
  },
  {
    area: "Deployments & Containers",
    items: [
      "Controlled deployment state machine with per-target atomic locking and idempotency keys",
      "Verified health checks prior to promotion and rollback to known previous artifacts",
      "Optional Unix-socket Docker Engine inspection with read-only defaults and mutation allowlists",
      "Read-only GitHub commits, workflow runs, and HMAC-SHA256 verified webhook ingestion",
    ],
  },
  {
    area: "Reliability & Incident Response",
    items: [
      "Alert rule engine with pending duration, cooldown anti-storm guards, and recovery notices",
      "Incident lifecycle (Open, Investigating, Identified, Monitoring, Resolved) with postmortems",
      "Structured log ingestion with automatic token and password redaction and CSV export",
      "Interactive SVG topology map plus accessible tabular relationship view",
    ],
  },
];

export default function FeaturesPage() {
  return (
    <PublicShell>
      <div className="mx-auto max-w-5xl space-y-12 px-6 py-14">
        <header className="max-w-3xl space-y-4">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            Engineered around operational fundamentals
          </h1>
          <p className="text-base text-[var(--text-secondary)]">
            Every screen inside the InfraOS workspace maps to implemented backend endpoints,
            relational tables, and server-side access checks.
          </p>
        </header>

        <div className="space-y-8">
          {capabilities.map((group) => (
            <section key={group.area} className="infra-surface p-6">
              <h2 className="text-lg font-semibold text-[var(--text-primary)]">{group.area}</h2>
              <ul className="mt-4 grid gap-3 sm:grid-cols-2">
                {group.items.map((text) => (
                  <li
                    key={text}
                    className="infra-surface-subtle p-3.5 text-sm leading-relaxed text-[var(--text-secondary)]"
                  >
                    {text}
                  </li>
                ))}
              </ul>
            </section>
          ))}
        </div>

        <div className="flex flex-wrap gap-3">
          <Link href="/demo" className="infra-btn-primary">
            Explore in demo workspace
          </Link>
          <Link href="/architecture" className="infra-btn-secondary">
            Review system architecture
          </Link>
        </div>
      </div>
    </PublicShell>
  );
}
