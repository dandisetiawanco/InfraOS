import type { Metadata } from "next";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";
import { API_BASE } from "@/lib/api";

export const metadata: Metadata = {
  title: "Documentation",
  description: "Local setup, agent enrollment, deployment profiles, backup procedures, and API reference for InfraOS.",
};

export default function DocsPage() {
  return (
    <PublicShell>
      <div className="mx-auto max-w-5xl space-y-10 px-6 py-14">
        <header className="max-w-3xl space-y-4">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            Engineering documentation
          </h1>
          <p className="text-base text-[var(--text-secondary)]">
            Quickstart instructions for running the lightweight local profile, enrolling the Go
            agent, verifying backups, and inspecting the live OpenAPI schema.
          </p>
        </header>

        <section className="infra-surface space-y-4 p-6">
          <h2 className="text-lg font-semibold">1. Start the lightweight local profile</h2>
          <p className="text-sm text-[var(--text-secondary)]">
            Runs the Go API with SQLite (WAL mode) and the Next.js frontend without requiring
            Docker or external database servers.
          </p>
          <pre className="overflow-x-auto rounded-xl bg-[var(--surface-subtle)] p-4 font-mono text-xs leading-relaxed">
{`make setup
make dev`}
          </pre>
        </section>

        <section className="infra-surface space-y-4 p-6">
          <h2 className="text-lg font-semibold">2. Enroll your local machine</h2>
          <p className="text-sm text-[var(--text-secondary)]">
            Create an account at <Link href="/register" className="underline">/register</Link>, open
            Hosts inside the workspace, generate a one-time enrollment token, and run the Go agent.
          </p>
          <pre className="overflow-x-auto rounded-xl bg-[var(--surface-subtle)] p-4 font-mono text-xs leading-relaxed">
{`INFRAOS_AGENT_ENROLL_TOKEN="<one-time-token>" \\
INFRAOS_AGENT_API_URL="${API_BASE}" \\
./bin/infraos-agent`}
          </pre>
        </section>

        <section className="infra-surface space-y-4 p-6">
          <h2 className="text-lg font-semibold">3. Consistent backup and temporary restore test</h2>
          <pre className="overflow-x-auto rounded-xl bg-[var(--surface-subtle)] p-4 font-mono text-xs leading-relaxed">
{`make backup-sqlite
make test-restore-sqlite`}
          </pre>
        </section>

        <section className="infra-surface space-y-3 p-6">
          <h2 className="text-lg font-semibold">4. Interactive REST API documentation</h2>
          <p className="text-sm text-[var(--text-secondary)]">
            When running locally with <code className="font-mono">INFRAOS_EXPOSE_API_DOCS=true</code>,
            the control plane serves both OpenAPI 3.1 JSON and an interactive route viewer.
          </p>
          <div className="flex flex-wrap gap-3 pt-1">
            <a
              href={`${API_BASE}/api/docs`}
              target="_blank"
              rel="noreferrer"
              className="infra-btn-primary"
            >
              Open API reference
            </a>
            <a
              href={`${API_BASE}/api/v1/openapi.json`}
              target="_blank"
              rel="noreferrer"
              className="infra-btn-secondary"
            >
              View OpenAPI JSON
            </a>
          </div>
        </section>
      </div>
    </PublicShell>
  );
}
