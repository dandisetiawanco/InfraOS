"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Menu, X } from "lucide-react";
import { BrandMark } from "./brand-mark";
import { ThemeToggle } from "./theme-toggle";

const navLinks = [
  { href: "/platform", label: "Platform" },
  { href: "/features", label: "Features" },
  { href: "/architecture", label: "Architecture" },
  { href: "/security", label: "Security" },
  { href: "/docs", label: "Documentation" },
  { href: "/demo", label: "Demo" },
];

export function PublicShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (mobileOpen && !dialog.open) {
      dialog.showModal();
    } else if (!mobileOpen && dialog.open) {
      dialog.close();
    }
  }, [mobileOpen]);

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);

  return (
    <div className="flex min-h-dvh flex-col bg-[var(--canvas)] text-[var(--text-primary)]">
      <header className="sticky top-0 z-30 px-4 pt-3 sm:px-6">
        <div className="infra-glass-nav mx-auto flex max-w-6xl items-center justify-between gap-4 rounded-2xl px-4 py-2.5 sm:px-5">
          <Link
            href="/"
            className="inline-flex items-center gap-2.5 text-base font-bold tracking-tight text-[var(--text-primary)]"
          >
            <BrandMark size={30} />
            <span>
              Infra<span className="text-[var(--accent)]">OS</span>
            </span>
          </Link>

          <nav aria-label="Primary" className="hidden items-center gap-1 lg:flex">
            {navLinks.map((item) => {
              const active = pathname === item.href;
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={`rounded-lg px-3 py-1.5 text-sm font-medium transition-colors ${
                    active
                      ? "bg-[var(--accent-soft)] text-[var(--text-primary)] underline decoration-[var(--accent)] decoration-2 underline-offset-4"
                      : "text-[var(--text-secondary)] hover:bg-[var(--surface-subtle)] hover:text-[var(--text-primary)]"
                  }`}
                >
                  {item.label}
                </Link>
              );
            })}
          </nav>

          <div className="flex items-center gap-2">
            <ThemeToggle variant="compact" />
            <Link href="/login" className="infra-btn-secondary hidden sm:inline-flex">
              Sign in
            </Link>
            <Link href="/demo" className="infra-btn-primary hidden sm:inline-flex">
              Try the demo
            </Link>
            <button
              type="button"
              onClick={() => setMobileOpen(true)}
              aria-label="Open menu"
              aria-expanded={mobileOpen}
              aria-controls="public-mobile-nav"
              className="infra-btn-secondary inline-flex px-3 lg:hidden"
            >
              <Menu className="h-4 w-4" aria-hidden="true" />
              <span>Menu</span>
            </button>
          </div>
        </div>
      </header>

      <dialog
        ref={dialogRef}
        id="public-mobile-nav"
        aria-label="Site navigation"
        onClose={() => setMobileOpen(false)}
        onClick={(e) => {
          if (e.target === dialogRef.current) setMobileOpen(false);
        }}
        className="infra-glass-overlay m-auto w-[min(92vw,380px)] rounded-2xl p-5 text-[var(--text-primary)]"
      >
        <div className="mb-4 flex items-center justify-between">
          <span className="inline-flex items-center gap-2 text-base font-bold">
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
        <nav aria-label="Mobile" className="flex flex-col gap-1">
          {navLinks.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              aria-current={pathname === item.href ? "page" : undefined}
              className="flex min-h-[44px] items-center rounded-lg px-3 text-sm font-medium text-[var(--text-primary)] hover:bg-[var(--surface-subtle)]"
            >
              {item.label}
            </Link>
          ))}
        </nav>
        <div className="mt-4 flex flex-col gap-2 border-t border-[var(--border)] pt-4">
          <Link href="/demo" className="infra-btn-primary w-full">
            Try the demo
          </Link>
          <Link href="/login" className="infra-btn-secondary w-full">
            Sign in to workspace
          </Link>
          <Link href="/register" className="infra-btn-secondary w-full">
            Create local account
          </Link>
        </div>
      </dialog>

      <main id="main-content" className="flex-1">
        {children}
      </main>

      <footer className="mt-20 border-t border-[var(--border)] bg-[var(--surface)]">
        <div className="mx-auto grid max-w-6xl gap-10 px-6 py-12 md:grid-cols-[1.4fr_1fr_1fr]">
          <div className="space-y-3">
            <div className="inline-flex items-center gap-2.5 text-base font-bold">
              <BrandMark size={26} />
              <span>
                Infra<span className="text-[var(--accent)]">OS</span>
              </span>
            </div>
            <p className="max-w-sm text-sm leading-relaxed text-[var(--text-secondary)]">
              Infrastructure operations platform built with Go and Next.js for real host
              telemetry, service checks, controlled deployments, and operational analysis.
            </p>
            <p className="text-xs text-[var(--text-secondary)]">
              Created by{" "}
              <a
                href="https://www.dandisetiawan.id/"
                target="_blank"
                rel="noreferrer"
                className="underline hover:text-[var(--text-primary)]"
              >
                Dandi Setiawan
              </a>
              . Source hosted on{" "}
              <a
                href="https://github.com/dandisetiawanco/InfraOS"
                target="_blank"
                rel="noreferrer"
                className="underline hover:text-[var(--text-primary)]"
              >
                GitHub
              </a>
              . All rights reserved.
            </p>
          </div>

          <div className="space-y-2.5">
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">Product</h2>
            <ul className="space-y-2 text-sm text-[var(--text-secondary)]">
              <li><Link href="/platform" className="hover:text-[var(--text-primary)]">Platform overview</Link></li>
              <li><Link href="/features" className="hover:text-[var(--text-primary)]">Operational capabilities</Link></li>
              <li><Link href="/architecture" className="hover:text-[var(--text-primary)]">System architecture</Link></li>
              <li><Link href="/security" className="hover:text-[var(--text-primary)]">Security boundaries</Link></li>
            </ul>
          </div>

          <div className="space-y-2.5">
            <h2 className="text-sm font-semibold text-[var(--text-primary)]">Workspace</h2>
            <ul className="space-y-2 text-sm text-[var(--text-secondary)]">
              <li><Link href="/demo" className="hover:text-[var(--text-primary)]">Isolated synthetic demo</Link></li>
              <li><Link href="/docs" className="hover:text-[var(--text-primary)]">Documentation</Link></li>
              <li><Link href="/login" className="hover:text-[var(--text-primary)]">Sign in</Link></li>
              <li><Link href="/register" className="hover:text-[var(--text-primary)]">Create local account</Link></li>
            </ul>
          </div>
        </div>
      </footer>
    </div>
  );
}
