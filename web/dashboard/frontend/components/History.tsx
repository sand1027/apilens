"use client";

import { useEffect, useState } from "react";
import { api, ApiError, decodeBase64, flattenHeaders, formatDuration, graphqlDisplay, isOverlayPoll, type Exchange } from "@/lib/api";
import { Button, Card, EmptyState, ErrorBanner, MethodBadge, StatusBadge } from "@/components/ui";

// History tab (plan.md v6): captured exchanges from the current or last
// `apilens watch` session — GET /api/history / GET /api/history/{id},
// identical to `apilens history list` / `apilens history show`.
export default function History({
  onOpenInRequestBuilder,
}: {
  onOpenInRequestBuilder: (displayID: number) => void;
}) {
  const [items, setItems] = useState<Exchange[]>([]);
  const [selected, setSelected] = useState<Exchange | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const res = (await api.historyList()).filter((ex) => !isOverlayPoll(ex));
      setItems(res);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to load history");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    // Initial data fetch on mount. react-hooks/set-state-in-effect flags
    // this as a potential cascading-render risk, but it's a known false
    // positive for async fetch-on-mount (facebook/react#34905) — setState
    // happens in the async continuation, not synchronously in the effect
    // body.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, []);

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_420px] gap-6 h-full">
      <div className="flex flex-col gap-4 min-w-0">
        <div className="flex items-center gap-2">
          <Button variant="secondary" onClick={load}>
            Refresh
          </Button>
          <span className="text-xs text-neutral-500">
            Session-scoped IDs — they reset each time `apilens watch` starts.
          </span>
        </div>
        {error && <ErrorBanner message={error} />}
        <Card className="p-0 overflow-hidden">
          {loading ? (
            <EmptyState>Loading...</EmptyState>
          ) : items.length === 0 ? (
            <EmptyState>
              No history yet. Run <code>apilens watch</code> and generate some traffic.
            </EmptyState>
          ) : (
            <table className="w-full text-sm">
              <tbody>
                {items.map((ex) => {
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
          <EmptyState>Select a captured exchange to see details.</EmptyState>
        ) : (
          <ExchangeDetail exchange={selected} onOpenInRequestBuilder={onOpenInRequestBuilder} />
        )}
      </Card>
    </div>
  );
}

export function ExchangeDetail({
  exchange,
  onOpenInRequestBuilder,
}: {
  exchange: Exchange;
  onOpenInRequestBuilder?: (displayID: number) => void;
}) {
  const reqHeaders = flattenHeaders(exchange.Request.Headers);
  const resHeaders = flattenHeaders(exchange.Response.Headers);
  const label = graphqlDisplay(exchange);
  return (
    <div className="flex flex-col gap-4 text-sm">
      <div className="flex items-center justify-between">
        <div className="flex flex-col gap-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="text-neutral-500 font-mono">#{exchange.Display}</span>
            <MethodBadge method={label.method} />
            <span className="font-mono text-xs truncate">{label.name}</span>
          </div>
          <span className="font-mono text-xs text-neutral-500 break-all">{exchange.Request.URL}</span>
        </div>
        {onOpenInRequestBuilder && (
          <Button variant="secondary" onClick={() => onOpenInRequestBuilder(exchange.Display)}>
            Replay
          </Button>
        )}
      </div>

      <div className="flex items-center gap-3">
        <StatusBadge status={exchange.Response.StatusCode} />
        <span className="text-neutral-500 text-xs">{formatDuration(exchange.Timing.Duration)}</span>
        {exchange.Redacted && <span className="text-xs text-yellow-500">redacted</span>}
      </div>

      <TimingWaterfall timing={exchange.Timing} />

      {exchange.Err && <ErrorBanner message={exchange.Err} />}

      <Section title="Request headers">
        <HeaderList pairs={reqHeaders} />
      </Section>
      {exchange.Request.Body && (
        <Section title="Request body">
          <pre className="text-xs bg-neutral-950 rounded p-2 overflow-auto max-h-40 whitespace-pre-wrap break-all">
            {decodeBase64(exchange.Request.Body)}
          </pre>
        </Section>
      )}
      <Section title="Response headers">
        <HeaderList pairs={resHeaders} />
      </Section>
      {exchange.Response.Body && (
        <Section title="Response body">
          <pre className="text-xs bg-neutral-950 rounded p-2 overflow-auto max-h-60 whitespace-pre-wrap break-all">
            {decodeBase64(exchange.Response.Body)}
            {exchange.Response.Truncated && "\n... (truncated)"}
          </pre>
        </Section>
      )}
    </div>
  );
}

export function TimingWaterfall({ timing }: { timing: Exchange["Timing"] }) {
  const phases = [
    { name: "dns", ns: timing.DNS ?? 0, color: "#38bdf8" },
    { name: "connect", ns: timing.Connect ?? 0, color: "#a78bfa" },
    { name: "tls", ns: timing.TLS ?? 0, color: "#f472b6" },
    { name: "wait", ns: timing.Wait ?? 0, color: "#fbbf24" },
    { name: "transfer", ns: timing.Transfer ?? 0, color: "#34d399" },
  ];
  const hasPhases = phases.some((p) => p.ns > 0);
  return (
    <div>
      <div className="text-xs text-neutral-500 mb-1">
        Timing{timing.TTFB ? ` · ttfb ${formatDuration(timing.TTFB)}` : ""}
      </div>
      {!hasPhases ? (
        <div className="text-xs text-neutral-600">
          Total {formatDuration(timing.Duration)}. Restart watch to capture DNS / connect / TLS / wait / transfer.
        </div>
      ) : (
        <div className="space-y-1">
          {phases.every((p) => p.name === "wait" || p.ns === 0) && (timing.Wait ?? 0) > 0 && (
            <div className="text-xs text-neutral-500 mb-1">
              Localhost HTTP + reused socket: DNS / connect / TLS skipped. Wait is server time.
            </div>
          )}
          {phases.map((p) => (
            <div key={p.name} className="flex items-center gap-2 text-xs font-mono">
              <span className="w-16 text-neutral-500">{p.name}</span>
              <div className="flex-1 h-2 bg-neutral-900 rounded overflow-hidden">
                <div
                  style={{
                    width: barWidth(p.ns, timing.Duration),
                    height: "100%",
                    background: p.color,
                  }}
                />
              </div>
              <span className="w-14 text-right text-neutral-400">{formatDuration(p.ns)}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function barWidth(part: number, total: number): string {
  if (part <= 0) return "0%";
  if (total <= 0) return "2%";
  return `${Math.max(2, Math.min(100, (part / total) * 100))}%`;
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-xs text-neutral-500 mb-1">{title}</div>
      {children}
    </div>
  );
}

function HeaderList({ pairs }: { pairs: [string, string][] }) {
  if (pairs.length === 0) return <div className="text-xs text-neutral-600">none</div>;
  return (
    <div className="font-mono text-xs space-y-0.5">
      {pairs.map(([k, v]) => (
        <div key={k} className="break-all">
          <span className="text-neutral-500">{k}:</span> {v}
        </div>
      ))}
    </div>
  );
}
