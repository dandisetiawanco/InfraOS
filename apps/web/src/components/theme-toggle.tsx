"use client";

import { useEffect, useState } from "react";
import { Monitor, Moon, Sun } from "lucide-react";

type SchemePreference = "system" | "light" | "dark";

function applyScheme(pref: SchemePreference) {
  const meta = document.querySelector('meta[name="color-scheme"]');
  const value = pref === "system" ? "light dark" : pref;
  if (meta) {
    meta.setAttribute("content", value);
  }
  try {
    if (pref === "system") {
      localStorage.removeItem("color-scheme");
    } else {
      localStorage.setItem("color-scheme", pref);
    }
  } catch {
    // Ignore blocked storage while still keeping the runtime scheme applied.
  }
}

export function ThemeToggle({ variant = "compact" }: { variant?: "compact" | "three-state" }) {
  const [pref, setPref] = useState<SchemePreference>("system");
  const [systemDark, setSystemDark] = useState(false);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    setSystemDark(mq.matches);
    const readStored = () => {
      try {
        const stored = localStorage.getItem("color-scheme");
        if (stored === "light" || stored === "dark") {
          setPref(stored);
          applyScheme(stored);
          return;
        }
      } catch {
        // Storage unavailable
      }
      setPref("system");
      applyScheme("system");
    };
    readStored();

    const onMQ = (e: MediaQueryListEvent) => setSystemDark(e.matches);
    const onStorage = (e: StorageEvent) => {
      if (e.key === "color-scheme") readStored();
    };
    mq.addEventListener("change", onMQ);
    window.addEventListener("storage", onStorage);
    return () => {
      mq.removeEventListener("change", onMQ);
      window.removeEventListener("storage", onStorage);
    };
  }, []);

  const effectiveDark = pref === "dark" || (pref === "system" && systemDark);

  const toggleCompact = () => {
    const target = effectiveDark ? "light" : "dark";
    const systemScheme = systemDark ? "dark" : "light";
    // Revert to system when selecting what already matches OS preference.
    const next: SchemePreference = target === systemScheme ? "system" : target;
    setPref(next);
    applyScheme(next);
  };

  if (variant === "three-state") {
    const options: Array<{ id: SchemePreference; label: string; icon: typeof Sun }> = [
      { id: "system", label: `System (${systemDark ? "dark" : "light"})`, icon: Monitor },
      { id: "light", label: "Light", icon: Sun },
      { id: "dark", label: "Dark", icon: Moon },
    ];
    return (
      <div className="inline-flex flex-wrap gap-2" role="radiogroup" aria-label="Color theme">
        {options.map(({ id, label, icon: Icon }) => {
          const active = pref === id;
          return (
            <button
              key={id}
              type="button"
              role="radio"
              aria-checked={active}
              onClick={() => {
                setPref(id);
                applyScheme(id);
              }}
              className={
                active
                  ? "infra-btn-primary"
                  : "infra-btn-secondary"
              }
            >
              <Icon className="h-4 w-4" aria-hidden="true" />
              <span>{label}</span>
            </button>
          );
        })}
      </div>
    );
  }

  return (
    <button
      type="button"
      onClick={toggleCompact}
      aria-label={effectiveDark ? "Switch to light theme" : "Switch to dark theme"}
      className="infra-touch-target inline-flex h-9 w-9 items-center justify-center rounded-lg border border-[var(--border-strong)] bg-[var(--surface)] text-[var(--text-primary)] hover:bg-[var(--surface-subtle)]"
    >
      {effectiveDark ? (
        <Sun className="h-4 w-4" aria-hidden="true" />
      ) : (
        <Moon className="h-4 w-4" aria-hidden="true" />
      )}
    </button>
  );
}
