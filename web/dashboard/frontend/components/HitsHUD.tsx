"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { api, formatDuration, graphqlDisplay, isOverlayPoll, type Exchange } from "@/lib/api";
import { Button, Card, EmptyState, MethodBadge, StatusBadge } from "@/components/ui";
import { ExchangeDetail } from "@/components/History";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";

/**
 * Always-on corner chip + modal (SDE-3 HUD). Lives on every dashboard tab.
 * Uses the same Engine history/watch APIs as the Runtime Monitor tab —
 * no extra backend. When watch is started from this UI process we also
 * subscribe to SSE; otherwise we poll history so a CLI `apilens watch`
 * in the same project still fills the widget.
 */
export default function HitsHUD({
  onOpenInRequestBuilder,
}: {
  onOpenInRequestBuilder: (displayID: number) => void;
}) {
  const [open, setOpen] = useState(false);
  const [running, setRunning] = useState(false);
  const [addr, setAddr] = useState<string | null>(null);
  const [events, setEvents] = useState<Exchange[]>([]);
  const [selected, setSelected] = useState<Exchange | null>(null);
  const esRef = useRef<EventSource | null>(null);
  const idsRef = useRef<Set<string>>(new Set());

  function upsert(ex: Exchange) {
    if (isOverlayPoll(ex)) return;
    const id = ex.ID || String(ex.Display);
    if (idsRef.current.has(id)) {
      setEvents((prev) => prev.map((e) => ((e.ID || String(e.Display)) === id ? ex : e)));
      return;
    }
    idsRef.current.add(id);
    setEvents((prev) => [ex, ...prev].slice(0, 500));
  }

  function connectStream() {
    if (esRef.current) return;
    const es = new EventSource(`${API_BASE}/api/watch/events`);
    es.onmessage = (evt) => {
      try {
        upsert(JSON.parse(evt.data) as Exchange);
      } catch {
        // ignore a malformed event
      }
    };
    esRef.current = es;
  }

  function disconnectStream() {
    esRef.current?.close();
    esRef.current = null;
  }

  async function refresh() {
    try {
      const status = await api.watchStatus();
      setRunning(status.running);
      setAddr(status.addr ?? null);
      if (status.running) connectStream();
      else disconnectStream();
    } catch {
      setRunning(false);
    }
    try {
      const hist = (await api.historyList(200)).filter((ex) => !isOverlayPoll(ex));
      setEvents((prev) => mergeExchanges(prev, hist));
      for (const ex of hist) {
        idsRef.current.add(ex.ID || String(ex.Display));
      }
    } catch {
      // history empty / unreadable is fine for the chip
    }
  }

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    refresh();
    const t = setInterval(refresh, 2000);
    return () => {
      clearInterval(t);
      disconnectStream();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const stats = useMemo(() => summarize(events), [events]);
  const last = events[0];
  const lastLabel = last ? graphqlDisplay(last) : null;

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex items-center gap-2 rounded-md bg-blue-600 hover:bg-blue-500 text-white px-3 py-1.5 text-sm font-medium"
        title="Live API hits"
      >
        <span className={`h-2 w-2 rounded-full ${running ? "bg-green-300" : "bg-white/50"}`} />
        Live hits
        <span className="font-mono text-xs bg-blue-800 rounded px-1.5 py-0.5">
          {stats.count}
        </span>
        {lastLabel && (
          <span className="font-mono text-xs max-w-[12rem] truncate hidden sm:inline">
            {lastLabel.method} {lastLabel.name}{" "}
            {lastLabel.gqlError ? "200*" : last?.Response.StatusCode}
          </span>
        )}
      </button>

      <button
        type="button"
        onClick={() => setOpen(true)}
        style={{
          position: "fixed",
          bottom: 16,
          right: 16,
          zIndex: 9999,
          display: "flex",
          alignItems: "center",
          gap: 8,
          borderRadius: 9999,
          border: "1px solid #3b82f6",
          background: "#1d4ed8",
          color: "#fff",
          padding: "8px 16px",
          boxShadow: "0 10px 25px rgba(0,0,0,0.45)",
          cursor: "pointer",
          textAlign: "left",
        }}
        title="Live API hits"
      >
        <span
          style={{
            height: 8,
            width: 8,
            borderRadius: 9999,
            background: running ? "#86efac" : "#93c5fd",
            display: "inline-block",
          }}
        />
        <span style={{ fontSize: 12 }}>
          {running ? `proxy ${addr}` : "proxy off"}
        </span>
        <span style={{ fontSize: 12, fontFamily: "ui-monospace, monospace" }}>
          {stats.count} hits
        </span>
        {lastLabel && (
          <span
            style={{
              fontSize: 12,
              fontFamily: "ui-monospace, monospace",
              maxWidth: "10rem",
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
          >
            {lastLabel.method} {lastLabel.name} {lastLabel.gqlError ? "200*" : last?.Response.StatusCode}{" "}
            {last ? formatDuration(last.Timing.Duration) : ""}
          </span>
        )}
      </button>

      {open && (
        <div
          className="fixed inset-0 flex items-end sm:items-center justify-center bg-black/60 p-4"
          style={{ zIndex: 10000 }}
        >
          <div className="w-full max-w-4xl max-h-[85vh] bg-neutral-950 border border-neutral-700 rounded-xl shadow-2xl flex flex-col overflow-hidden">
            <div className="px-4 py-3 border-b border-neutral-800 flex items-center justify-between gap-3">
              <div>
                <h2 className="text-sm font-semibold">Live API hits</h2>
                <p className="text-xs text-neutral-500">
                  {running ? `Listening on ${addr}` : "Start watch (CLI or Runtime Monitor) to capture."}{" "}
                  Health {stats.ok}/{stats.count} ok
                  {stats.count > 0 && (
                    <>
                      {" "}
                      · p50 {formatDuration(stats.p50)} · p95 {formatDuration(stats.p95)}
                    </>
                  )}
                </p>
              </div>
              <Button variant="secondary" onClick={() => setOpen(false)}>
                Close
              </Button>
            </div>
            <div className="grid grid-cols-1 md:grid-cols-[minmax(0,1fr)_340px] min-h-0 flex-1 overflow-hidden">
              <div className="overflow-auto">
                {events.length === 0 ? (
                  <EmptyState>
                    No hits yet. Run <code>apilens watch --browser</code>, then use the app.
                  </EmptyState>
                ) : (
                  <table className="w-full text-sm">
                    <tbody>
                      {events.map((ex) => {
                        const label = graphqlDisplay(ex);
                        return (
                          <tr
                            key={ex.ID || ex.Display}
                            onClick={() => setSelected(ex)}
                            className={`cursor-pointer border-b border-neutral-800 hover:bg-neutral-800/50 ${
                              selected?.ID === ex.ID ? "bg-neutral-800" : ""
                            }`}
                          >
                            <td className="px-3 py-2 w-10 text-neutral-500 font-mono">#{ex.Display}</td>
                            <td className="px-3 py-2 w-24">
                              <MethodBadge method={label.method} />
                            </td>
                            <td className="px-3 py-2 font-mono truncate max-w-[14rem]">{label.name}</td>
                            <td className="px-3 py-2 w-16">
                              {label.gqlError ? (
                                <span className="font-mono text-sm text-yellow-400">
                                  {ex.Response.StatusCode}*
                                </span>
                              ) : (
                                <StatusBadge status={ex.Response.StatusCode} />
                              )}
                            </td>
                            <td className="px-3 py-2 w-16 text-neutral-500 text-xs">
                              {formatDuration(ex.Timing.Duration)}
                            </td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}
              </div>
              <Card className="overflow-auto rounded-none border-0 border-l border-neutral-800">
                {!selected ? (
                  <EmptyState>Select a hit.</EmptyState>
                ) : (
                  <ExchangeDetail exchange={selected} onOpenInRequestBuilder={onOpenInRequestBuilder} />
                )}
              </Card>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function mergeExchanges(prev: Exchange[], incoming: Exchange[]): Exchange[] {
  const seen = new Set(prev.map((e) => e.ID || String(e.Display)));
  const extra: Exchange[] = [];
  for (const ex of incoming) {
    const id = ex.ID || String(ex.Display);
    if (!seen.has(id)) {
      seen.add(id);
      extra.push(ex);
    }
  }
  if (extra.length === 0) return prev;
  return [...extra.reverse(), ...prev].slice(0, 500);
}

function summarize(events: Exchange[]) {
  const durs = events.map((e) => e.Timing.Duration).filter((n) => n > 0);
  let ok = 0;
  for (const ex of events) {
    const label = graphqlDisplay(ex);
    if (ex.Response.StatusCode >= 200 && ex.Response.StatusCode < 400 && !label.gqlError) ok++;
  }
  return {
    count: events.length,
    ok,
    p50: percentile(durs, 50),
    p95: percentile(durs, 95),
  };
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const s = [...values].sort((a, b) => a - b);
  const idx = Math.min(s.length - 1, Math.max(0, Math.ceil((p / 100) * s.length) - 1));
  return s[idx];
}
