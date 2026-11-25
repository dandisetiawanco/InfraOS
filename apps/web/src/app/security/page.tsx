import type { Metadata } from "next";
import Link from "next/link";
import { PublicShell } from "@/components/public-shell";

export const metadata: Metadata = {
  title: "Security",
  description: "Authentication, RBAC, SSRF prevention, credential storage, and demo isolation in InfraOS.",
};

const controls = [
  {
    name: "Authentication & RBAC",
    detail:
      "Passwords are hashed with Argon2id. Sessions use CSPRNG tokens stored only as SHA-256 digests, paired with HttpOnly cookies, CSRF tokens, login rate limiting, and server-side role enforcement (Owner, Admin, Operator, Viewer).",
  },
  {
    name: "Agent authentication isolation",
    detail:
      "Agents authenticate separately via one-time expiring enrollment tokens exchanged for per-agent hashed credentials. Tokens can be rotated or revoked immediately without affecting user sessions.",
  },
  {
    name: "SSRF destination policy",
    detail:
      "Service probes and alert webhooks reject cloud metadata endpoints, link-local ranges, RFC1918 private networks, and non-allowlisted loopback addresses across both initial DNS resolution and HTTP redirects.",
  },
  {
    name: "No arbitrary command execution",
    detail:
      "The API never accepts raw shell commands or scripts. Deployment targets run predefined adapters, and optional Docker inspection stays read-only unless explicitly allowlisted.",
  },
  {
    name: "Public demonstration isolation",
    detail:
      "Demo sessions provision isolated synthetic organizations that are blocked server-side from accessing host metrics, local Docker sockets, external network probes, or real deployments.",
  },
  {
    name: "Credential redaction & webhooks",
    detail:
      "Log ingestion and optional AI summaries redact bearer tokens, API keys, DSN passwords, and private keys. GitHub webhooks verify HMAC-SHA256 signatures in constant time and reject duplicate delivery IDs.",
  },
];

export default function SecurityPage() {
  return (
    <PublicShell>
      <div className="mx-auto max-w-5xl space-y-10 px-6 py-14">
        <header className="max-w-3xl space-y-4">
          <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">
            Security architecture and threat boundaries
          </h1>
          <p className="text-base text-[var(--text-secondary)]">
            Security controls are enforced in the Go API layer and verified with automated tests.
            InfraOS makes no unverified compliance certification claims.
          </p>
        </header>

        <div className="grid gap-5 md:grid-cols-2">
          {controls.map((c) => (
            <section key={c.name} className="infra-surface space-y-2 p-6">
              <h2 className="text-base font-semibold">{c.name}</h2>
              <p className="text-sm leading-relaxed text-[var(--text-secondary)]">{c.detail}</p>
            </section>
          ))}
        </div>

        <div className="flex flex-wrap gap-3">
          <Link href="/demo" className="infra-btn-primary">
            Try the isolated sandbox
          </Link>
          <Link href="/architecture" className="infra-btn-secondary">
            System architecture
          </Link>
        </div>
      </div>
    </PublicShell>
  );
}
