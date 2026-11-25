import type { Metadata } from "next";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "Platform",
  description: "How InfraOS unifies host monitoring, service health checks, deployments, logs, and incidents.",
};

export default function PlatformPage() {
  return (
    <PublicShell>
      <div className="mx-auto max-w-5xl space-y-12 px-6 py-14">
        <header className="max-w-3xl space-y-4">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            One control plane for everyday infrastructure operations
          </h1>
          <p className="text-base leading-relaxed text-[var(--text-secondary)]">
            InfraOS combines host telemetry collection, scheduled endpoint probes, controlled
            release workflows, structured logs, alerts, and incident tracking into a single
            self-hosted Go and Next.js application.
          </p>
        </header>

        <section className="grid gap-6 md:grid-cols-2">
          {[
            {
              title: "Host & agent lifecycle",
              body: "Enroll macOS or Linux hosts with short-lived one-time tokens. Rotate credentials, inspect filesystem and interface details, and revoke compromised agents immediately.",
            },
            {
              title: "Observed service availability",
              body: "Run HTTP, HTTPS, TCP, DNS, and TLS expiration probes on bounded worker pools. Availability percentages are calculated strictly from recorded checks.",
            },
            {
              title: "Safe container visibility",
              body: "Inspect local Docker Engine containers in read-only mode when explicitly enabled. Mutations require server-side RBAC, explicit allowlists, and confirmation.",
            },
            {
              title: "Structured logs & operational analysis",
              body: "Ingest JSON logs with automatic credential redaction, filter across severity and correlation IDs, and review deterministic anomaly findings.",
            },
          ].map((item) => (
            <article key={item.title} className="infra-surface space-y-2.5 p-6">
              <h2 className="text-lg font-semibold">{item.title}</h2>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">{item.body}</p>
            </article>
          ))}
        </section>

        <div className="flex flex-wrap gap-3">
          <Link href="/demo" className="infra-btn-primary">
            Try the isolated demo
          </Link>
          <Link href="/features" className="infra-btn-secondary">
            Inspect full capability list
          </Link>
        </div>
      </div>
    </PublicShell>
  );
}
