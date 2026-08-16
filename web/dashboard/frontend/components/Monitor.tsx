"use client";

import { useEffect, useRef, useState } from "react";
import { api, ApiError, formatDuration, graphqlDisplay, type Exchange } from "@/lib/api";
import { Button, Card, EmptyState, ErrorBanner, Input, MethodBadge, StatusBadge } from "@/components/ui";
import { ExchangeDetail } from "@/components/History";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE ?? "";

// Runtime Monitor tab (plan.md v6): starts/stops the local proxy
// (POST /api/watch/start|stop, same as `apilens watch`) and streams
// captured exchanges live via Server-Sent Events
// (GET /api/watch/events) — the dashboard's only genuinely "live" view.
export default function Monitor({
  onOpenInRequestBuilder,
}: {
  onOpenInRequestBuilder: (displayID: number) => void;
}) {
  const [running, setRunning] = useState(false);
  const [addr, setAddr] = useState<string | null>(null);
  const [port, setPort] = useState("8888");
  const [events, setEvents] = useState<Exchange[]>([]);
  const [selected, setSelected] = useState<Exchange | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  async function refreshStatus() {
    try {
      const status = await api.watchStatus();
      setRunning(status.running);
      setAddr(status.addr ?? null);
      if (status.running && !esRef.current) connectStream();
      if (!status.running && esRef.current) disconnectStream();
    } catch {
      // status endpoint failing is non-fatal for the initial render
    }
  }

  useEffect(() => {
    // Initial status fetch on mount — same known false positive as
    // History.tsx's load() (facebook/react#34905): setState happens in
    // the async continuation, not synchronously in the effect body.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    refreshStatus();
    return () => disconnectStream();
    // refreshStatus/disconnectStream are stable for the component's
    // lifetime (no props/state captured); intentionally mount-only.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function connectStream() {
    const es = new EventSource(`${API_BASE}/api/watch/events`);
    es.onmessage = (evt) => {
      try {
        const ex: Exchange = JSON.parse(evt.data);
        setEvents((prev) => [ex, ...prev].slice(0, 500));
      } catch {
        // ignore a malformed event rather than breaking the whole stream
      }
    };
    esRef.current = es;
  }

  function disconnectStream() {
    esRef.current?.close();
    esRef.current = null;
  }

  async function start() {
    setBusy(true);
    try {
      const portNum = parseInt(port, 10);
      const res = await api.watchStart({ port: Number.isNaN(portNum) ? undefined : portNum });
      setAddr(res.addr);
      setRunning(true);
      setError(null);
      connectStream();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to start watch");
    } finally {
      setBusy(false);
    }
  }

  async function stop() {
    setBusy(true);
    try {
      await api.watchStop();
      setRunning(false);
      setAddr(null);
      disconnectStream();
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to stop watch");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_420px] gap-6 h-full">
      <div className="flex flex-col gap-4 min-w-0">
        <div className="flex items-center gap-2">
          {!running ? (
            <>
              <Input value={port} onChange={setPort} placeholder="Port" className="w-24" />
              <Button onClick={start} disabled={busy}>
                {busy ? "Starting..." : "Start watch"}
              </Button>
            </>
          ) : (
            <>
              <span className="text-sm text-green-400">● Listening on {addr}</span>
              <span className="text-xs text-neutral-500">
                Set HTTP_PROXY=http://{addr} in the app you want to observe.
              </span>
              <Button variant="danger" onClick={stop} disabled={busy}>
                Stop
              </Button>
            </>
          )}
        </div>

        {error && <ErrorBanner message={error} />}

        <Card className="p-0 overflow-hidden flex-1">
          {events.length === 0 ? (
            <EmptyState>
              {running
                ? "Waiting for traffic through the proxy..."
                : "Start watch, then point traffic at the proxy address."}
            </EmptyState>
          ) : (
            <table className="w-full text-sm">
              <tbody>
                {events.map((ex) => {
                  const label = graphqlDisplay(ex);
                  return (
                  <tr
                    key={ex.ID}
                    onClick={() => setSelected(ex)}
                    className={`cursor-pointer border-b border-neutral-800 last:border-0 hover:bg-neutral-800/50 ${
                      selected?.ID === ex.ID ? "bg-neutral-800" : ""
                    }`}
                  >
                    <td className="px-4 py-2 w-12 text-neutral-500 font-mono">#{ex.Display}</td>
                    <td className="px-4 py-2 w-24">
                      <MethodBadge method={label.method} />
                    </td>
                    <td className="px-4 py-2 font-mono text-neutral-200 truncate max-w-md">{label.name}</td>
                    <td className="px-4 py-2 w-16">
                      <StatusBadge status={ex.Response.StatusCode} />
                    </td>
                    <td className="px-4 py-2 w-20 text-neutral-500 text-xs">
                      {formatDuration(ex.Timing.Duration)}
                    </td>
                  </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </Card>
      </div>

      <Card className="overflow-auto">
        {!selected ? (
          <EmptyState>Select a live exchange to see details.</EmptyState>
        ) : (
          <ExchangeDetail exchange={selected} onOpenInRequestBuilder={onOpenInRequestBuilder} />
        )}
      </Card>
    </div>
  );
}
