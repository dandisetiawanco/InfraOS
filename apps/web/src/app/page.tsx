import Link from "next/link";
import { PublicShell } from "@/components/public-shell";
import { HeroWorkspacePreview } from "@/components/hero-preview";

export default function HomePage() {
  return (
    <PublicShell>
      {/* Hero */}
      <section className="mx-auto max-w-6xl px-6 pb-12 pt-14 sm:pt-20">
        <div className="max-w-3xl space-y-6">
          <h1 className="text-4xl font-bold tracking-tight text-[var(--text-primary)] sm:text-5xl lg:text-6xl">
            Infrastructure under control.
          </h1>
          <p className="max-w-2xl text-lg leading-relaxed text-[var(--text-secondary)]">
            Monitor your systems, understand operational events, and manage deployments from one
            connected workspace.
          </p>
          <div className="flex flex-wrap items-center gap-3 pt-2">
            <Link href="/platform" className="infra-btn-primary px-5 py-2.5">
              Explore InfraOS
            </Link>
            <Link href="/demo" className="infra-btn-secondary px-5 py-2.5">
              Try the demo
            </Link>
            <Link
              href="/register"
              className="px-3 py-2 text-sm font-medium text-[var(--text-secondary)] underline underline-offset-4 hover:text-[var(--text-primary)]"
            >
              Connect your local machine
            </Link>
          </div>
        </div>

        <div className="mt-10">
          <HeroWorkspacePreview />
        </div>
      </section>

      {/* Section 2: Asymmetric editorial composition (Real host collection) */}
      <section className="mx-auto max-w-6xl px-6 py-16">
        <div className="grid items-start gap-10 lg:grid-cols-[1.1fr_1fr]">
          <div className="space-y-4">
            <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">
              Real operating system telemetry without root privileges
            </h2>
            <p className="text-base leading-relaxed text-[var(--text-secondary)]">
              The InfraOS Go agent runs directly on authorized macOS and Linux machines. It reads
              CPU load, memory availability, mounted filesystem capacity, and network throughput
              deltas while guarding against counter resets and buffering samples during brief
              network outages.
            </p>
            <ul className="space-y-2.5 pt-2 text-sm text-[var(--text-primary)]">
              <li className="flex gap-3">
                <span className="font-mono text-[var(--accent)]">•</span>
                <span>One-time expiring enrollment tokens exchanged for per-agent hashed credentials.</span>
              </li>
              <li className="flex gap-3">
                <span className="font-mono text-[var(--accent)]">•</span>
                <span>Every chart labels data freshness: live, stale, historical, unavailable, or synthetic.</span>
              </li>
              <li className="flex gap-3">
                <span className="font-mono text-[var(--accent)]">•</span>
                <span>Optional battery and thermal readings appear only when the host exposes them.</span>
              </li>
            </ul>
          </div>

          <div className="infra-surface divide-y divide-[var(--border)]">
            <div className="px-5 py-4">
              <div className="text-xs font-semibold text-[var(--text-secondary)]">Agent pipeline</div>
              <div className="mt-1 text-sm font-medium">Bounded collection, exponential backoff with jitter</div>
            </div>
            <div className="grid grid-cols-2 divide-x divide-[var(--border)] text-sm">
              <div className="p-5">
                <div className="text-xs text-[var(--text-secondary)]">Primary backend</div>
                <div className="mt-1 font-mono font-semibold">Go + Chi + slog</div>
              </div>
              <div className="p-5">
                <div className="text-xs text-[var(--text-secondary)]">Live transport</div>
                <div className="mt-1 font-mono font-semibold">SSE + polling fallback</div>
              </div>
            </div>
            <div className="grid grid-cols-2 divide-x divide-[var(--border)] text-sm">
              <div className="p-5">
                <div className="text-xs text-[var(--text-secondary)]">Auth separation</div>
                <div className="mt-1 font-mono font-semibold">Argon2id + Agent tokens</div>
              </div>
              <div className="p-5">
                <div className="text-xs text-[var(--text-secondary)]">Storage engines</div>
                <div className="mt-1 font-mono font-semibold">SQLite &amp; PostgreSQL</div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Section 3: Full-width operational chain */}
      <section className="border-y border-[var(--border)] bg-[var(--surface)] py-16">
        <div className="mx-auto max-w-6xl px-6">
          <div className="max-w-2xl space-y-3">
            <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">
              From probe observation to verified recovery
            </h2>
            <p className="text-base text-[var(--text-secondary)]">
              Monitoring checks, alert evaluation, controlled deployments, and incident timelines
              share one database schema and correlation model.
            </p>
          </div>

          <div className="mt-10 grid gap-8 md:grid-cols-2">
            <div className="space-y-3 border-l-2 border-[var(--accent)] pl-5">
              <h3 className="text-base font-semibold">SSRF-hardened service monitoring</h3>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">
                A bounded worker pool executes HTTP, HTTPS, TCP, DNS, and TLS expiration checks.
                Every hostname, resolved IP, and HTTP redirect is validated against destination
                policies that block cloud metadata endpoints and unauthorized private networks.
              </p>
            </div>

            <div className="space-y-3 border-l-2 border-[var(--border-strong)] pl-5">
              <h3 className="text-base font-semibold">Controlled deployments with target locks</h3>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">
                Idempotency keys and database-backed target locks prevent overlapping rollouts.
                Releases progress through explicit states and verify target health before marking
                success or restoring a previous artifact on rollback.
              </p>
            </div>

            <div className="space-y-3 border-l-2 border-[var(--border-strong)] pl-5">
              <h3 className="text-base font-semibold">Alert cooldowns and deduplicated incidents</h3>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">
                Alert rules evaluate duration thresholds and cooldown windows to prevent alert
                storms. Rules marked for automatic escalation open a single deduplicated incident
                with linked timeline updates.
              </p>
            </div>

            <div className="space-y-3 border-l-2 border-[var(--border-strong)] pl-5">
              <h3 className="text-base font-semibold">Deterministic insights before optional AI</h3>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">
                Built-in analyzers flag sustained memory pressure, rising response latency, stale
                heartbeats, and deployment timing correlations without ever claiming unverified
                root cause. External AI summarization stays disabled by default.
              </p>
            </div>
          </div>
        </div>
      </section>

      {/* Section 4: Deployment profiles comparison table */}
      <section className="mx-auto max-w-6xl px-6 py-16">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="max-w-2xl space-y-2">
            <h2 className="text-2xl font-bold tracking-tight sm:text-3xl">
              Three profiles matched to your hardware
            </h2>
            <p className="text-sm text-[var(--text-secondary)]">
              Run the lightweight profile on an 8 GB laptop with zero containers, or switch to
              PostgreSQL, Redis, Nginx, and Prometheus when running the infrastructure stack.
            </p>
          </div>
          <Link href="/architecture" className="infra-btn-secondary">
            Read system architecture
          </Link>
        </div>

        <div className="infra-surface infra-table-wrap mt-6">
          <table className="infra-table infra-table-stack">
            <thead>
              <tr>
                <th scope="col">Profile</th>
                <th scope="col">Database &amp; queue</th>
                <th scope="col">Runtime requirements</th>
                <th scope="col">Intended environment</th>
              </tr>
            </thead>
            <tbody>
              <tr>
                <th scope="row" className="font-semibold">Lightweight</th>
                <td data-label="Database & queue">SQLite (WAL) + in-process worker pool</td>
                <td data-label="Runtime requirements">Go + Node.js only (no Docker required)</td>
                <td data-label="Intended environment">Local macOS or Linux workstation</td>
              </tr>
              <tr>
                <th scope="row" className="font-semibold">Infrastructure</th>
                <td data-label="Database & queue">PostgreSQL 16 + Redis 7</td>
                <td data-label="Runtime requirements">Docker Compose, Nginx, optional Prometheus</td>
                <td data-label="Intended environment">Local container stack and staging</td>
              </tr>
              <tr>
                <th scope="row" className="font-semibold">Production</th>
                <td data-label="Database & queue">PostgreSQL + Redis + verified backups</td>
                <td data-label="Runtime requirements">Authorized Linux host, TLS proxy, systemd agent</td>
                <td data-label="Intended environment">Dedicated Linux operations host</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      {/* Section 5: Direct closing strip */}
      <section className="mx-auto max-w-6xl px-6 pb-8">
        <div className="infra-surface flex flex-col items-start justify-between gap-6 p-8 md:flex-row md:items-center">
          <div className="max-w-xl space-y-2">
            <h2 className="text-xl font-bold tracking-tight sm:text-2xl">
              Inspect the sandbox or run locally on your machine
            </h2>
            <p className="text-sm text-[var(--text-secondary)]">
              Public demo sessions run in isolated synthetic organizations and cannot access real
              host sockets or external targets.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            <Link href="/demo" className="infra-btn-primary">
              Launch isolated demo
            </Link>
            <Link href="/docs" className="infra-btn-secondary">
              View documentation
            </Link>
          </div>
        </div>
      </section>
    </PublicShell>
  );
}
