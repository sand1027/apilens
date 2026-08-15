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
  Request?: { Method?: string; URL?: string; Body?: string };
  Response?: { StatusCode?: number; Body?: string };
  Timing?: { Duration?: number };
};

const UI_CANDIDATES = ["http://127.0.0.1:4488", "http://localhost:4488"];

function Chip() {
  const [open, setOpen] = useState(false);
  const [events, setEvents] = useState<Exchange[]>([]);
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
        setEvents(
          data
            .filter((ex: Exchange) => !isOverlayPoll(ex))
            .slice()
            .reverse(),
        );
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
              width: "min(900px,100%)",
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
                      ? `${events.length} hits`
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
            <div style={{ overflow: "auto" }}>
              {events.length === 0 ? (
                <div style={{ padding: 24, color: "#737373" }}>
                  No product API hits yet. Restart watch with a fresh Chrome window:
                  apilens watch --browser --open http://localhost:3001
                </div>
              ) : (
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <tbody>
                    {events.map((ex, i) => {
                      const l = gqlLabel(ex);
                      const st = l.gqlError
                        ? String(ex.Response?.StatusCode ?? "") + "*"
                        : String(ex.Response?.StatusCode ?? "");
                      return (
                        <tr key={ex.ID || String(ex.Display) + "-" + i} style={{ borderBottom: "1px solid #262626" }}>
                          <td style={{ padding: "6px 8px", color: "#737373", fontFamily: "ui-monospace,monospace" }}>
                            #{ex.Display}
                          </td>
                          <td style={{ padding: "6px 8px", fontFamily: "ui-monospace,monospace" }}>{l.method}</td>
                          <td
                            style={{
                              padding: "6px 8px",
                              fontFamily: "ui-monospace,monospace",
                              maxWidth: 280,
                              overflow: "hidden",
                              textOverflow: "ellipsis",
                              whiteSpace: "nowrap",
                            }}
                          >
                            {l.name}
                          </td>
                          <td style={{ padding: "6px 8px", fontFamily: "ui-monospace,monospace" }}>{st}</td>
                          <td style={{ padding: "6px 8px", color: "#737373" }}>{ms(ex)}ms</td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  );
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

function ms(ex: Exchange) {
  const d = ex.Timing?.Duration;
  if (!d) return 0;
  return Math.round(d / 1e6);
}
