"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";

/** Injected by apilens init. Live-hits overlay in this app. Does not change URLs. */
export default function ApiLensWidget() {
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (!mounted || typeof document === "undefined") return null;
  return createPortal(<Chip />, document.body);
}

type Exchange = {
  ID?: string;
  Display?: number;
  Request?: {
    Method?: string;
    URL?: string;
    Body?: string;
    Headers?: Record<string, string[]>;
  };
  Response?: {
    StatusCode?: number;
    Body?: string;
    Headers?: Record<string, string[]>;
    Truncated?: boolean;
  };
  Timing?: {
    Duration?: number;
    DNS?: number;
    Connect?: number;
    TLS?: number;
    Wait?: number;
    TTFB?: number;
    Transfer?: number;
  };
  Err?: string | null;
};

const UI_CANDIDATES = ["http://127.0.0.1:4488", "http://localhost:4488"];

function Chip() {
  const [open, setOpen] = useState(false);
  const [events, setEvents] = useState<Exchange[]>([]);
  const [selected, setSelected] = useState<Exchange | null>(null);
  const [running, setRunning] = useState(false);
  const [addr, setAddr] = useState("");
  const [ui, setUi] = useState(UI_CANDIDATES[0]);
  const [reachable, setReachable] = useState(false);

  useEffect(() => {
    let stop = false;
    async function tick() {
      let base = ui;
      let ok = false;
      for (const candidate of UI_CANDIDATES) {
        try {
          const st = await fetch(candidate + "/api/watch/status", { cache: "no-store" });
          if (!st.ok) continue;
          const body = await st.json();
          if (stop) return;
          setRunning(!!body.running);
          setAddr(body.addr || "");
          base = candidate;
          ok = true;
          break;
        } catch {
          // try next
        }
      }
      if (stop) return;
      setReachable(ok);
      setUi(base);
      if (!ok) return;
      try {
        const hist = await fetch(base + "/api/history?limit=80", { cache: "no-store" });
        const data = await hist.json();
        if (stop || !Array.isArray(data)) return;
        const next = data
          .filter((ex: Exchange) => !isOverlayPoll(ex))
          .slice()
          .reverse();
        setEvents(next);
        setSelected((cur) => {
          if (!cur) return next[0] ?? null;
          return next.find((ex) => (ex.ID || ex.Display) === (cur.ID || cur.Display)) ?? next[0] ?? null;
        });
      } catch {
        // keep chip visible
      }
    }
    tick();
    const id = setInterval(tick, 2000);
    return () => {
      stop = true;
      clearInterval(id);
    };
  }, [ui]);

  const last = events[0];
  const label = last ? gqlLabel(last) : null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        style={{
          position: "fixed",
          left: 16,
          bottom: 16,
          zIndex: 2147483647,
          display: "flex",
          alignItems: "center",
          gap: 8,
          borderRadius: 9999,
          border: "1px solid #3b82f6",
          background: "#1d4ed8",
          color: "#fff",
          padding: "10px 16px",
          cursor: "pointer",
          boxShadow: "0 10px 25px rgba(0,0,0,0.45)",
          font: "13px/1.3 system-ui,sans-serif",
        }}
      >
        <span
          style={{
            height: 8,
            width: 8,
            borderRadius: 9999,
            background: running ? "#86efac" : reachable ? "#93c5fd" : "#fca5a5",
            display: "inline-block",
          }}
        />
        Live hits
        <span style={{ fontFamily: "ui-monospace,monospace" }}>{events.length}</span>
        {label && (
          <span
            style={{
              fontFamily: "ui-monospace,monospace",
              maxWidth: "12rem",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {label.method} {label.name}
          </span>
        )}
        {!reachable && <span style={{ opacity: 0.85 }}>start apilens ui</span>}
      </button>
      {open && (
        <div
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false);
          }}
          style={{
            position: "fixed",
            inset: 0,
            zIndex: 2147483647,
            background: "rgba(0,0,0,0.55)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            padding: 16,
          }}
        >
          <div
            style={{
              width: "min(1100px,100%)",
              maxHeight: "85vh",
              background: "#0a0a0a",
              color: "#f5f5f5",
              border: "1px solid #404040",
              borderRadius: 12,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
              fontFamily: "system-ui,sans-serif",
            }}
          >
            <div
              style={{
                padding: "12px 16px",
                borderBottom: "1px solid #262626",
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
              }}
            >
              <div>
                <div style={{ fontWeight: 600, fontSize: 14 }}>Live API hits</div>
                <div style={{ fontSize: 12, color: "#a3a3a3" }}>
                  {!reachable
                    ? "apilens ui is not running on :4488"
                    : events.length > 0
                      ? `${events.length} hits · click a row for headers, body, and timings`
                      : running
                        ? `proxy ${addr} · waiting for traffic`
                        : "In the API repo: apilens watch --browser --open http://localhost:3001"}
                </div>
              </div>
              <button
                type="button"
                onClick={() => setOpen(false)}
                style={{
                  background: "#262626",
                  color: "#fff",
                  border: 0,
                  borderRadius: 6,
                  padding: "6px 10px",
                  cursor: "pointer",
                }}
              >
                Close
              </button>
            </div>
            {events.length === 0 ? (
              <div style={{ padding: 24, color: "#737373" }}>
                No product API hits yet. Restart watch with a fresh Chrome window:
                apilens watch --browser --open http://localhost:3001
              </div>
            ) : (
              <div style={{ display: "grid", gridTemplateColumns: "minmax(0,1fr) 380px", minHeight: 0, flex: 1 }}>
                <div style={{ overflow: "auto", borderRight: "1px solid #262626" }}>
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <tbody>
                      {events.map((ex, i) => {
                        const l = gqlLabel(ex);
                        const st = l.gqlError
                          ? String(ex.Response?.StatusCode ?? "") + "*"
                          : String(ex.Response?.StatusCode ?? "");
                        const active = (selected?.ID || selected?.Display) === (ex.ID || ex.Display);
                        return (
                          <tr
                            key={ex.ID || String(ex.Display) + "-" + i}
                            onClick={() => setSelected(ex)}
                            style={{
                              borderBottom: "1px solid #262626",
                              cursor: "pointer",
                              background: active ? "#1e3a5f" : "transparent",
                            }}
                          >
                            <td style={{ padding: "6px 8px", color: "#737373", fontFamily: "ui-monospace,monospace" }}>
                              #{ex.Display}
                            </td>
                            <td style={{ padding: "6px 8px", fontFamily: "ui-monospace,monospace" }}>{l.method}</td>
                            <td
                              style={{
                                padding: "6px 8px",
                                fontFamily: "ui-monospace,monospace",
                                maxWidth: 220,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                              }}
                            >
                              {l.name}
                            </td>
                            <td style={{ padding: "6px 8px", fontFamily: "ui-monospace,monospace" }}>{st}</td>
                            <td style={{ padding: "6px 8px", color: "#737373" }}>{fmtNS(ex.Timing?.Duration)}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                </div>
                <div style={{ overflow: "auto", padding: 12 }}>{selected ? <HitDetail ex={selected} /> : null}</div>
              </div>
            )}
          </div>
        </div>
      )}
    </>
  );
}

function HitDetail({ ex }: { ex: Exchange }) {
  const l = gqlLabel(ex);
  const st = l.gqlError ? String(ex.Response?.StatusCode ?? "") + "*" : String(ex.Response?.StatusCode ?? "");
  return (
    <div style={{ fontSize: 12, display: "flex", flexDirection: "column", gap: 12 }}>
      <div>
        <div style={{ fontFamily: "ui-monospace,monospace", fontWeight: 600 }}>
          #{ex.Display} {l.method} {l.name}
        </div>
        <div style={{ color: "#a3a3a3", fontFamily: "ui-monospace,monospace", wordBreak: "break-all" }}>
          {ex.Request?.Method} {ex.Request?.URL}
        </div>
        <div style={{ color: "#a3a3a3", marginTop: 4 }}>
          {st} · {fmtNS(ex.Timing?.Duration)}
          {ex.Err ? ` · ${ex.Err}` : ""}
        </div>
      </div>
      <TimingBars timing={ex.Timing} />
      <Block title="Request headers">{headerText(ex.Request?.Headers)}</Block>
      <Block title="Request body">{prettyBody(ex.Request?.Body)}</Block>
      <Block title="Response headers">{headerText(ex.Response?.Headers)}</Block>
      <Block title="Response body">
        {prettyBody(ex.Response?.Body)}
        {ex.Response?.Truncated ? "\n... (truncated)" : ""}
      </Block>
    </div>
  );
}

function TimingBars({ timing }: { timing?: Exchange["Timing"] }) {
  const phases = [
    { name: "dns", ns: timing?.DNS ?? 0, color: "#38bdf8" },
    { name: "connect", ns: timing?.Connect ?? 0, color: "#a78bfa" },
    { name: "tls", ns: timing?.TLS ?? 0, color: "#f472b6" },
    { name: "wait", ns: timing?.Wait ?? 0, color: "#fbbf24" },
    { name: "transfer", ns: timing?.Transfer ?? 0, color: "#34d399" },
  ];
  const total = timing?.Duration ?? 0;
  const has = phases.some((p) => p.ns > 0);
  return (
    <div>
      <div style={{ color: "#737373", marginBottom: 6 }}>
        Timing{timing?.TTFB ? ` · ttfb ${fmtNS(timing.TTFB)}` : ""}
      </div>
      {!has ? (
        <div style={{ color: "#525252" }}>
          Total {fmtNS(total)}. Restart watch to capture DNS / connect / TLS / wait / transfer.
        </div>
      ) : (
        <>
          {networkSkipped(phases) && (
            <div style={{ color: "#737373", marginBottom: 8 }}>
              Localhost HTTP and a reused socket — DNS / connect / TLS do not run again. Wait is API
              time (resolver + DB). Transfer stays ~0ms when the JSON fits in the first packet.
            </div>
          )}
          {phases.map((p) => (
            <div key={p.name} style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 4 }}>
              <span style={{ width: 64, color: "#737373", fontFamily: "ui-monospace,monospace" }}>{p.name}</span>
              <div style={{ flex: 1, height: 8, background: "#171717", borderRadius: 4, overflow: "hidden" }}>
                <div
                  style={{
                    width: barPct(p.ns, total),
                    height: "100%",
                    background: p.color,
                  }}
                />
              </div>
              <span style={{ width: 56, textAlign: "right", color: "#a3a3a3", fontFamily: "ui-monospace,monospace" }}>
                {fmtNS(p.ns)}
              </span>
            </div>
          ))}
        </>
      )}
    </div>
  );
}

function Block({ title, children }: { title: string; children: string }) {
  return (
    <div>
      <div style={{ color: "#737373", marginBottom: 4 }}>{title}</div>
      <pre
        style={{
          margin: 0,
          background: "#111",
          borderRadius: 6,
          padding: 8,
          maxHeight: 160,
          overflow: "auto",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
          fontFamily: "ui-monospace,monospace",
          fontSize: 11,
          color: children ? "#e5e5e5" : "#525252",
        }}
      >
        {children || "none"}
      </pre>
    </div>
  );
}

function networkSkipped(phases: { name: string; ns: number }[]) {
  const wait = phases.find((p) => p.name === "wait")?.ns ?? 0;
  const net = phases.filter((p) => p.name !== "wait").reduce((s, p) => s + p.ns, 0);
  return wait > 0 && net === 0;
}

function barPct(part: number, total: number) {
  if (part <= 0) return "0%";
  if (total <= 0) return "2%";
  return `${Math.max(2, Math.min(100, (part / total) * 100))}%`;
}

function fmtNS(ns?: number) {
  if (!ns) return "0ms";
  const ms = ns / 1e6;
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

function headerText(h?: Record<string, string[]>) {
  if (!h) return "";
  return Object.entries(h)
    .map(([k, v]) => `${k}: ${v.join(", ")}`)
    .join("\n");
}

function prettyBody(raw?: string) {
  const s = b64(raw);
  if (!s) return "";
  if (/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/.test(s)) {
    return "Compressed body (gzip/br). Restart apilens watch to store decoded JSON.";
  }
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

function b64(s?: string) {
  if (!s) return "";
  try {
    return atob(s);
  } catch {
    return "";
  }
}

function isOverlayPoll(ex: Exchange) {
  const raw = ex.Request?.URL || "";
  try {
    const u = new URL(raw);
    if (u.port !== "4488") return false;
    return (
      u.pathname === "/api/watch/status" ||
      u.pathname.startsWith("/api/watch/") ||
      u.pathname === "/api/history" ||
      u.pathname.startsWith("/api/history/") ||
      u.pathname === "/widget.js"
    );
  } catch {
    return raw.includes(":4488/") && (raw.includes("/api/watch") || raw.includes("/api/history"));
  }
}

function gqlLabel(ex: Exchange) {
  const body = b64(ex.Request?.Body);
  try {
    const payload = JSON.parse(body) as { query?: string; operationName?: string };
    const q = (payload.query || "").trim();
    if (q) {
      const method = /^\s*mutation\b/i.test(q)
        ? "MUTATION"
        : /^\s*subscription\b/i.test(q)
          ? "SUBSCRIPTION"
          : "QUERY";
      const named = (payload.operationName || "").trim();
      const field = (q.match(/\{\s*([A-Za-z_][\w]*)/) || [])[1] || "anonymous";
      let gqlError = false;
      try {
        const env = JSON.parse(b64(ex.Response?.Body)) as { errors?: unknown[] };
        gqlError = Array.isArray(env.errors) && env.errors.length > 0;
      } catch {
        gqlError = false;
      }
      return { method, name: named || field, gqlError };
    }
  } catch {
    // not GraphQL
  }
  return {
    method: ex.Request?.Method || "GET",
    name: ex.Request?.URL || "",
    gqlError: false,
  };
}
