"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { PublicShell } from "@/components/public-shell";
import { apiFetch, APIError } from "@/lib/api";

const loginSchema = z.object({
  email: z.string().email("Enter a valid email address."),
  password: z.string().min(8, "Password must be at least 8 characters."),
});

type LoginForm = z.infer<typeof loginSchema>;

export default function LoginPage() {
  const router = useRouter();
  const [serverError, setServerError] = useState("");
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginForm>({ resolver: zodResolver(loginSchema) });

  async function onSubmit(values: LoginForm) {
    setServerError("");
    try {
      await apiFetch("/api/v1/auth/login", { method: "POST", json: values });
      router.push("/workspace");
    } catch (err) {
      setServerError(err instanceof APIError ? err.message : "Sign in failed.");
    }
  }

  return (
    <PublicShell>
      <div className="mx-auto max-w-md px-6 py-16">
        <div className="infra-surface space-y-6 p-7">
          <header className="space-y-1.5">
            <h1 className="text-2xl font-bold tracking-tight">Sign in to InfraOS</h1>
            <p className="text-sm text-[var(--text-secondary)]">
              Access your local or organization infrastructure workspace.
            </p>
          </header>

          {serverError ? (
            <div
              role="alert"
              className="rounded-lg border border-[var(--status-err)] bg-[var(--status-err-bg)] px-3.5 py-2.5 text-sm text-[var(--status-err)]"
            >
              {serverError}
            </div>
          ) : null}

          <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
            <div className="space-y-1.5">
              <label htmlFor="login-email" className="block text-sm font-medium">
                Email address
              </label>
              <input
                id="login-email"
                type="email"
                autoComplete="email"
                placeholder="email@example.com"
                className="infra-input"
                aria-invalid={Boolean(errors.email)}
                aria-describedby={errors.email ? "login-email-err" : undefined}
                {...register("email")}
              />
              {errors.email ? (
                <p id="login-email-err" className="text-xs text-[var(--status-err)]">
                  {errors.email.message}
                </p>
              ) : null}
            </div>

            <div className="space-y-1.5">
              <label htmlFor="login-password" className="block text-sm font-medium">
                Password
              </label>
              <input
                id="login-password"
                type="password"
                autoComplete="current-password"
                placeholder="Enter your password"
                className="infra-input"
                aria-invalid={Boolean(errors.password)}
                aria-describedby={errors.password ? "login-password-err" : undefined}
                {...register("password")}
              />
              {errors.password ? (
                <p id="login-password-err" className="text-xs text-[var(--status-err)]">
                  {errors.password.message}
                </p>
              ) : null}
            </div>

            <button type="submit" disabled={isSubmitting} className="infra-btn-primary w-full">
              {isSubmitting ? "Signing in..." : "Sign in"}
            </button>
          </form>

          <div className="flex flex-wrap items-center justify-between gap-2 border-t border-[var(--border)] pt-4 text-sm text-[var(--text-secondary)]">
            <Link href="/register" className="underline hover:text-[var(--text-primary)]">
              Create an account
            </Link>
            <Link href="/demo" className="underline hover:text-[var(--text-primary)]">
              Launch synthetic demo
            </Link>
          </div>
        </div>
      </div>
    </PublicShell>
  );
}
