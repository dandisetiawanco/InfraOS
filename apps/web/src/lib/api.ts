export const API_BASE = (
  process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080"
).replace(/\/+$/, "");

export function getClientApiBase(): string {
  if (typeof window !== "undefined" && API_BASE) {
    try {
      const url = new URL(API_BASE);
      const pageHost = window.location.hostname;
      const isLoopback = (h: string) => h === "localhost" || h === "127.0.0.1";
      if (isLoopback(url.hostname) && isLoopback(pageHost) && url.hostname !== pageHost) {
        url.hostname = pageHost;
        return url.toString().replace(/\/+$/, "");
      }
    } catch {
      // Keep relative or non-standard API_BASE unchanged.
    }
  }
  return API_BASE;
}

let cachedCSRF = "";

export function setCSRFToken(token: string) {
  if (token) cachedCSRF = token;
}

function readCookie(name: string): string {
  if (typeof document === "undefined") return "";
  const match = document.cookie.match(new RegExp("(?:^|; )" + name + "=([^;]*)"));
  return match ? decodeURIComponent(match[1]) : "";
}

export class APIError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function apiFetch<T>(
  path: string,
  init: RequestInit & { json?: unknown } = {},
): Promise<T> {
  const { json, headers, ...rest } = init;
  const h = new Headers(headers);
  if (json !== undefined) {
    h.set("Content-Type", "application/json");
  }
  const method = (rest.method || "GET").toUpperCase();
  if (method !== "GET" && method !== "HEAD") {
    const csrf = cachedCSRF || readCookie("infraos_csrf");
    if (csrf) {
      h.set("X-CSRF-Token", csrf);
    }
  }

  const res = await fetch(`${getClientApiBase()}${path}`, {
    ...rest,
    method,
    headers: h,
    credentials: "include",
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });

  const text = await res.text();
  let parsed: { data?: T; error?: { code?: string; message?: string } } = {};
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = {};
    }
  }

  if (!res.ok) {
    throw new APIError(
      res.status,
      parsed.error?.code || "request_failed",
      parsed.error?.message || `Request failed with HTTP ${res.status}`,
    );
  }

  const data = parsed.data as T;
  if (data && typeof data === "object" && "csrf_token" in data) {
    const token = (data as Record<string, unknown>).csrf_token;
    if (typeof token === "string") setCSRFToken(token);
  }
  return data;
}

export async function apiFetchWithMeta<T, M = Record<string, unknown>>(
  path: string,
): Promise<{ data: T; meta?: M }> {
  const res = await fetch(`${getClientApiBase()}${path}`, {
    credentials: "include",
  });
  const parsed = await res.json();
  if (!res.ok) {
    throw new APIError(
      res.status,
      parsed.error?.code || "request_failed",
      parsed.error?.message || `Request failed with HTTP ${res.status}`,
    );
  }
  return { data: parsed.data as T, meta: parsed.meta as M };
}
