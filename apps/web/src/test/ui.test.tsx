import { describe, expect, it, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { StatusBadge, DataFreshnessTag, StateView } from "@/components/status-badge";
import { ThemeToggle } from "@/components/theme-toggle";
import { HeroWorkspacePreview } from "@/components/hero-preview";
import { formatBytes, formatBps, formatUptime } from "@/lib/format";

describe("Formatting utilities", () => {
  it("formats bytes, throughput, and uptime cleanly", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1024 * 1024 * 2)).toBe("2.0 MiB");
    expect(formatBps(2048)).toBe("2.0 KiB/s");
    expect(formatUptime(0)).toBe("Unavailable");
    expect(formatUptime(90061)).toBe("1d 1h");
  });
});

describe("Status and data-state components", () => {
  it("renders status badges and freshness labels truthfully", () => {
    render(
      <div>
        <StatusBadge status="online" />
        <DataFreshnessTag source="synthetic" />
      </div>,
    );
    expect(screen.getByText("online")).toBeInTheDocument();
    expect(screen.getByText("Synthetic demo data")).toBeInTheDocument();
  });

  it("renders loading, empty, and error UI states with accessible roles", () => {
    const { rerender } = render(
      <StateView kind="loading" title="Loading hosts" description="Fetching telemetry" />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Loading hosts");

    rerender(<StateView kind="empty" title="No hosts" description="Enroll an agent" />);
    expect(screen.getByRole("status")).toHaveTextContent("No hosts");

    rerender(<StateView kind="error" title="API unreachable" description="Check port 8080" />);
    expect(screen.getByRole("alert")).toHaveTextContent("API unreachable");
  });
});

describe("ThemeToggle", () => {
  beforeEach(() => {
    localStorage.clear();
    let meta = document.querySelector('meta[name="color-scheme"]');
    if (!meta) {
      meta = document.createElement("meta");
      meta.setAttribute("name", "color-scheme");
      document.head.appendChild(meta);
    }
    meta.setAttribute("content", "light dark");
  });

  it("switches meta color-scheme and persists override", () => {
    render(<ThemeToggle variant="three-state" />);
    fireEvent.click(screen.getByRole("radio", { name: "Dark" }));
    expect(document.querySelector('meta[name="color-scheme"]')?.getAttribute("content")).toBe("dark");
    expect(localStorage.getItem("color-scheme")).toBe("dark");

    fireEvent.click(screen.getByRole("radio", { name: "Light" }));
    expect(document.querySelector('meta[name="color-scheme"]')?.getAttribute("content")).toBe("light");
    expect(localStorage.getItem("color-scheme")).toBe("light");
  });
});

describe("HeroWorkspacePreview", () => {
  it("switches between interactive tabs without fabricated live claims", () => {
    render(<HeroWorkspacePreview />);
    expect(screen.getByText("Synthetic sample")).toBeInTheDocument();
    expect(screen.getByText("Connected hosts (sample topology)")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Deployments" }));
    expect(screen.getByText("Controlled release pipeline with target locking")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Incidents" }));
    expect(screen.getByText("Incident timeline & deterministic correlation")).toBeInTheDocument();
  });
});
