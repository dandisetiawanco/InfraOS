"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { zodResolver } from "@hookform/resolvers/zod";
import { PublicShell } from "@/components/public-shell";
import { apiFetch, APIError } from "@/lib/api";

const registerSchema = z.object({
  name: z.string().min(2, "Enter your name."),
  email: z.string().email("Enter a valid email address."),
  organization_name: z.string().min(2, "Enter an organization or workspace name."),
  password: z.string().min(8, "Password must be at least 8 characters."),
});

type RegisterForm = z.infer<typeof registerSchema>;

export default function RegisterPage() {
  const router = useRouter();
  const [serverError, setServerError] = useState("");
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<RegisterForm>({ resolver: zodResolver(registerSchema) });

  async function onSubmit(values: RegisterForm) {
    setServerError("");
    try {
      await apiFetch("/api/v1/auth/register", { method: "POST", json: values });
      router.push("/workspace");
    } catch (err) {
      setServerError(err instanceof APIError ? err.message : "Registration failed.");
    }
  }

  return (
    <PublicShell>
      <div className="mx-auto max-w-md px-6 py-16">
        <div className="infra-surface space-y-6 p-7">
          <header className="space-y-1.5">
            <h1 className="text-2xl font-bold tracking-tight">Create local workspace</h1>
            <p className="text-sm text-[var(--text-secondary)]">
              Provisions an authenticated owner account ready to enroll real macOS or Linux hosts.
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
              <label htmlFor="reg-name" className="block text-sm font-medium">
                Your name
              </label>
              <input
                id="reg-name"
                type="text"
                autoComplete="name"
                placeholder="Your Name"
                className="infra-input"
                aria-invalid={Boolean(errors.name)}
                {...register("name")}
              />
              {errors.name ? (
                <p className="text-xs text-[var(--status-err)]">{errors.name.message}</p>
              ) : null}
            </div>

            <div className="space-y-1.5">
              <label htmlFor="reg-org" className="block text-sm font-medium">
                Organization name
              </label>
              <input
                id="reg-org"
                type="text"
                placeholder="Local Engineering Workspace"
                className="infra-input"
                aria-invalid={Boolean(errors.organization_name)}
                {...register("organization_name")}
              />
              {errors.organization_name ? (
                <p className="text-xs text-[var(--status-err)]">{errors.organization_name.message}</p>
              ) : null}
            </div>

            <div className="space-y-1.5">
              <label htmlFor="reg-email" className="block text-sm font-medium">
                Email address
              </label>
              <input
                id="reg-email"
                type="email"
                autoComplete="email"
                placeholder="email@example.com"
                className="infra-input"
                aria-invalid={Boolean(errors.email)}
                {...register("email")}
              />
              {errors.email ? (
                <p className="text-xs text-[var(--status-err)]">{errors.email.message}</p>
              ) : null}
            </div>

            <div className="space-y-1.5">
              <label htmlFor="reg-password" className="block text-sm font-medium">
                Password
              </label>
              <span id="reg-pw-hint" className="block text-xs text-[var(--text-secondary)]">
                Minimum 8 characters. Hashed server-side with Argon2id.
              </span>
              <input
                id="reg-password"
                type="password"
                autoComplete="new-password"
                placeholder="Create a strong password"
                aria-describedby="reg-pw-hint"
                className="infra-input"
                aria-invalid={Boolean(errors.password)}
                {...register("password")}
              />
              {errors.password ? (
                <p className="text-xs text-[var(--status-err)]">{errors.password.message}</p>
              ) : null}
            </div>

            <button type="submit" disabled={isSubmitting} className="infra-btn-primary w-full">
              {isSubmitting ? "Creating workspace..." : "Create workspace"}
            </button>
          </form>

          <div className="border-t border-[var(--border)] pt-4 text-sm text-[var(--text-secondary)]">
            Already have an account?{" "}
            <Link href="/login" className="underline hover:text-[var(--text-primary)]">
              Sign in
            </Link>
          </div>
        </div>
      </div>
    </PublicShell>
  );
}
