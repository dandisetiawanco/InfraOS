"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";
import { apiFetch, APIError } from "@/lib/api";

export default function DemoPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleLaunchDemo() {
    setLoading(true);
    setError("");
    try {
      await apiFetch("/api/v1/auth/demo", { method: "POST" });
      router.push("/workspace");
    } catch (err) {
      setError(
        err instanceof APIError
          ? err.message
          : "Unable to reach the InfraOS API. Ensure the backend server is running.",
      );
      setLoading(false);
    }
  }

  return (
    <PublicShell>
      <div className="mx-auto max-w-3xl space-y-8 px-6 py-16">
        <header className="space-y-3">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            Isolated synthetic demonstration
          </h1>
          <p className="text-base leading-relaxed text-[var(--text-secondary)]">
            Launch a private sandbox organization pre-populated with clearly labeled synthetic
            hosts, metrics history, service checks, containers, simulation deployments, logs,
            alerts, and an active incident timeline.
          </p>
        </header>

        <section className="infra-surface space-y-4 p-6">
          <h2 className="text-base font-semibold">Sandbox boundaries</h2>
          <ul className="space-y-2 text-sm text-[var(--text-secondary)]">
            <li>• Isolated per session: actions stay inside your temporary demo organization.</li>
            <li>• Read-only host protection: cannot inspect the real local machine or Docker socket.</li>
            <li>• Network safety: cannot configure arbitrary external probe targets or webhooks.</li>
            <li>• Simulation deployments only: never runs shell scripts or modifies live servers.</li>
          </ul>

          {error ? (
            <div
              role="alert"
              className="rounded-lg border border-[var(--status-err)] bg-[var(--status-err-bg)] px-4 py-3 text-sm text-[var(--status-err)]"
            >
              {error}
            </div>
          ) : null}

          <div className="flex flex-wrap items-center gap-3 pt-2">
            <button
              type="button"
              onClick={handleLaunchDemo}
              disabled={loading}
              className="infra-btn-primary px-6"
            >
              {loading ? "Provisioning sandbox..." : "Launch isolated demo workspace"}
            </button>
            <Link href="/register" className="infra-btn-secondary">
              Create real local account instead
            </Link>
          </div>
        </section>
      </div>
    </PublicShell>
  );
}
