// Thin fetch wrapper around the Go backend's JSON REST API
// (internal/webapi). This file intentionally contains NO business logic —
// it only shapes HTTP calls and TypeScript types that mirror the Go
// domain/pkg types. Every action here maps 1:1 to an Engine method the
// CLI can already perform (plan.md v6's non-negotiable).

export type Endpoint = {
  ID: string;
  Method: string;
  Path: string;
  Sources: string[];
  PrimarySource: string;
  Tags: string[] | null;
  Spec: {
    Name: string;
    Parameters: { Name: string; In: string; Required: boolean; Type: string }[] | null;
    RequestBody: unknown;
    Responses: unknown;
  } | null;
};

export type Exchange = {
  ID: string;
  Display: number;
  Request: {
    Method: string;
    URL: string;
    Headers: Record<string, string[]> | null;
    Query: Record<string, string[]> | null;
    Body: string; // base64, per Go's []byte JSON encoding
  };
  Response: {
    StatusCode: number;
    Headers: Record<string, string[]> | null;
    Body: string; // base64
    Truncated: boolean;
  };
  Timing: {
    Start: string;
    End: string;
    Duration: number; // nanoseconds (Go time.Duration)
  };
  Timestamp: string;
  Redacted: boolean;
  Err: string | null;
};

export type AssertionResult = {
  Kind: string;
  Target: string;
  Passed: boolean;
  Expected: string;
  Actual: string;
  Reason: string;
};

export type TestResult = {
  Name: string;
  File: string;
  Status: "passed" | "failed" | "errored" | "skipped";
  Method: string;
  URL: string;
  HTTPStatus: number;
  DurationMS: number;
  Assertions: AssertionResult[] | null;
  Error: string;
};

export type Report = {
  Env: string;
  Counts: { tests: number; passed: number; failed: number; errored: number; skipped: number };
  DurationMS: number;
  Results: TestResult[] | null;
  GeneratedAt: string;
};

export type Environment = {
  Name: string;
  BaseURL: string;
  Variables: Record<string, string> | null;
};

export type Inspection = {
  Endpoint: Endpoint;
  Live: Exchange | null;
};

export type GeneratedTest = {
  Path: string;
  Content: string; // base64
  Test: unknown;
};

class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

// In production (the embedded static export served by `apilens ui`), the
// frontend and the API are same-origin, so this stays empty. During
// `npm run dev` against a separately-running `apilens ui`, set
// NEXT_PUBLIC_API_BASE=http://127.0.0.1:4488 so the dev server (on its own
// port) can reach the Go backend.
const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  });
  if (!res.ok) {
    let message = res.statusText;
    try {
      const body = await res.json();
      if (body?.error) message = body.error;
    } catch {
      // response wasn't JSON; fall back to statusText
    }
    throw new ApiError(res.status, message);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  listEndpoints: (filter?: { method?: string; path?: string; tag?: string; source?: string }) => {
    const q = new URLSearchParams();
    if (filter?.method) q.set("method", filter.method);
    if (filter?.path) q.set("path", filter.path);
    if (filter?.tag) q.set("tag", filter.tag);
    if (filter?.source) q.set("source", filter.source);
    const qs = q.toString();
    return request<Endpoint[]>(`/api/endpoints${qs ? `?${qs}` : ""}`);
  },

  discover: (opts?: { source?: string[]; path?: string[] }) =>
    request<{ Endpoints: Endpoint[]; Errors: { Provider: string; Message: string }[] | null }>(
      "/api/discover",
      { method: "POST", body: JSON.stringify(opts ?? {}) }
    ),

  inspect: (ref: string, opts?: { method?: string; live?: boolean }) => {
    const q = new URLSearchParams({ ref });
    if (opts?.method) q.set("method", opts.method);
    if (opts?.live) q.set("live", "true");
    return request<Inspection>(`/api/inspect?${q.toString()}`);
  },

  run: (filter?: {
    ref?: string;
    method?: string;
    tag?: string;
    sequential?: boolean;
    parallel?: boolean;
    failFast?: boolean;
  }) => request<Report>("/api/run", { method: "POST", body: JSON.stringify(filter ?? {}) }),

  historyList: (limit?: number) =>
    request<Exchange[]>(`/api/history${limit ? `?limit=${limit}` : ""}`),

  historyGet: (id: number) => request<Exchange>(`/api/history/${id}`),

  replay: (
    id: number,
    overrides?: {
      method?: string;
      url?: string;
      headers?: Record<string, string>;
      unset?: string[];
      query?: Record<string, string>;
    }
  ) =>
    request<Exchange>(`/api/replay/${id}`, {
      method: "POST",
      body: JSON.stringify(overrides ?? {}),
    }),

  generate: (id: number, opts?: { out?: string; force?: boolean }) =>
    request<GeneratedTest>(`/api/generate/${id}`, {
      method: "POST",
      body: JSON.stringify(opts ?? {}),
    }),

  listEnvironments: () => request<{ current: string; items: Environment[] }>("/api/environments"),

  useEnvironment: (name: string) =>
    request<Environment>("/api/environments/use", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),

  watchStart: (opts?: {
    bind?: string;
    port?: number;
    allowRemote?: boolean;
    upstream?: string;
    pathPrefix?: string;
    host?: string;
    all?: boolean;
  }) => request<{ addr: string }>("/api/watch/start", { method: "POST", body: JSON.stringify(opts ?? {}) }),

  watchStop: () => request<{ stopped: boolean }>("/api/watch/stop", { method: "POST" }),

  watchStatus: () => request<{ running: boolean; addr?: string }>("/api/watch/status"),
};

export { ApiError };

/** Decode a base64 string (as Go emits for []byte) to UTF-8 text. */
export function decodeBase64(b64: string): string {
  if (!b64) return "";
  try {
    return decodeURIComponent(
      atob(b64)
        .split("")
        .map((c) => "%" + c.charCodeAt(0).toString(16).padStart(2, "0"))
        .join("")
    );
  } catch {
    return atob(b64);
  }
}

/** Format a Go time.Duration (nanoseconds) as "12ms" / "1.2s". */
export function formatDuration(ns: number): string {
  const ms = ns / 1e6;
  if (ms < 1000) return `${ms.toFixed(0)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

/** Flatten Go's http.Header-shaped map (name -> string[]) into name:value pairs. */
export function flattenHeaders(h: Record<string, string[]> | null | undefined): [string, string][] {
  if (!h) return [];
  return Object.entries(h).map(([k, v]) => [k, v.join(", ")]);
}
