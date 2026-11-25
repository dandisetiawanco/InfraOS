import { test, expect } from "@playwright/test";
import path from "path";

const SCREENSHOT_DIR = path.resolve(__dirname, "../../../assets/screenshots");
const API_URL = process.env.INFRAOS_E2E_API_URL || "http://127.0.0.1:8080";
const LIVE_EMAIL = process.env.INFRAOS_E2E_LIVE_EMAIL || "local-owner@infraos.local";
const LIVE_PASS = process.env.INFRAOS_E2E_LIVE_PASSWORD || "InfraOSLocal2026!";

test.describe("InfraOS End-to-End Verification & Screenshots", () => {
  test("public landing, responsive breakpoints, demo workspace, and full screenshot capture", async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    // 1. Public landing page (1440px)
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/");
    await expect(page.getByRole("heading", { level: 1, name: "Infrastructure under control." })).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "01-landing-page.png"), fullPage: false });

    // Verify interactive hero tabs
    await page.getByRole("tab", { name: "Deployments" }).click();
    await expect(page.getByText("Controlled release pipeline with target locking")).toBeVisible();

    // Verify no horizontal overflow across all 5 required viewports
    for (const width of [390, 768, 1024, 1440, 1920]) {
      await page.setViewportSize({ width, height: 844 });
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
      expect(overflow, `horizontal overflow detected at ${width}px`).toBe(false);
    }

    // Return to desktop 1440px and launch isolated synthetic demo
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/demo");
    await page.getByRole("button", { name: "Launch isolated demo workspace" }).click();
    await page.waitForURL("**/workspace");
    await expect(page.getByText("Isolated synthetic demo mode:")).toBeVisible();
    await expect(page.getByRole("heading", { level: 1, name: "Infrastructure overview" })).toBeVisible();
    await page.waitForTimeout(400);
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "02-workspace-overview.png") });

    // 4. Host details (synthetic demo host with full historical series)
    await page.goto("/workspace/hosts");
    await expect(page.getByText("edge-api-01 (Demo)")).toBeVisible();
    await page.getByRole("link", { name: "edge-api-01 (Demo)" }).click();
    await page.waitForURL("**/workspace/hosts/*");
    await expect(page.getByText("CPU and memory utilization over time")).toBeVisible();
    await page.waitForTimeout(500);
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "04-host-details.png") });

    // 5. Service monitoring
    await page.goto("/workspace/services");
    await expect(page.getByText("Edge API Gateway (Demo)")).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "05-service-monitoring.png") });

    // 6. Containers (synthetic demo records clearly labeled)
    await page.goto("/workspace/containers");
    await expect(page.getByText("infraos-edge-proxy")).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "06-containers.png") });

    // 7. Deployments
    await page.goto("/workspace/deployments");
    await expect(page.getByText("v2.14.2")).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "07-deployments.png") });

    // 8. Logs filtering
    await page.goto("/workspace/logs");
    await expect(page.getByRole("heading", { level: 1, name: "Centralized logs" })).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "08-logs.png") });

    // 9. Alerts
    await page.goto("/workspace/alerts");
    await expect(page.getByText("High Worker Memory Utilization")).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "09-alerts.png") });

    // 10. Incidents workflow
    await page.goto("/workspace/incidents");
    await page.getByText("Staging DNS Resolver check timeouts").click();
    await page.waitForURL("**/workspace/incidents/*");
    await expect(page.getByText("Operational timeline")).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "10-incident-workspace.png") });

    // 11. Topology
    await page.goto("/workspace/topology");
    await expect(page.getByRole("img", { name: "Configured infrastructure topology graph" })).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "11-topology.png") });

    // 12. Command Palette (Cmd+K / toolbar trigger)
    await page.getByRole("button", { name: "Open command palette" }).click();
    await expect(page.getByPlaceholder(/Jump to hosts/i)).toBeVisible();
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "12-command-palette.png") });
    await page.keyboard.press("Escape");

    // 13. Dark mode verification on overview
    await page.goto("/workspace/settings");
    await page.getByRole("radio", { name: "Dark" }).click();
    await page.goto("/workspace");
    await page.waitForTimeout(300);
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "13-dark-mode.png") });

    // 14. Mobile experience (390px) with responsive menu
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto("/workspace");
    await page.waitForTimeout(300);
    const mobileOverflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth + 1);
    expect(mobileOverflow).toBe(false);
    await page.screenshot({ path: path.join(SCREENSHOT_DIR, "14-mobile-workspace.png") });

    // Switch back to Light and sign in as the real local owner account to capture real macOS host monitoring
    await page.setViewportSize({ width: 1440, height: 900 });
    await page.goto("/workspace/settings");
    await page.getByRole("radio", { name: "Light" }).click();

    const loginCheck = await page.request.post(`${API_URL}/api/v1/auth/login`, {
      data: { email: LIVE_EMAIL, password: LIVE_PASS },
    });
    if (loginCheck.ok()) {
      await page.goto("/login");
      await page.fill("#login-email", LIVE_EMAIL);
      await page.fill("#login-password", LIVE_PASS);
      await page.getByRole("button", { name: "Sign in" }).click();
      await page.waitForURL("**/workspace");
      await page.goto("/workspace/hosts");
      await expect(page.getByText("Live host telemetry").or(page.getByText("online"))).toBeVisible();
      await page.waitForTimeout(400);
      await page.screenshot({ path: path.join(SCREENSHOT_DIR, "03-real-local-host-monitoring.png") });
    }

    expect(consoleErrors.filter((e) => !e.includes("favicon"))).toEqual([]);
  });
});
