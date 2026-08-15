"use client";

import { useEffect, useState } from "react";
import { api, ApiError, type Endpoint, type Inspection } from "@/lib/api";
import { Button, Card, EmptyState, ErrorBanner, Input, MethodBadge, StatusBadge } from "@/components/ui";

// API Explorer tab (plan.md v6). Lists the registry (GET /api/endpoints,
// same as `apilens list`) and can trigger discovery (POST /api/discover,
// same as `apilens discover`). Selecting an endpoint shows its spec via
// GET /api/inspect, with an optional live probe — identical to
// `apilens inspect <ref> [--live]`.
export default function Explorer({ onInspectLive }: { onInspectLive: (displayID: number) => void }) {
  const [endpoints, setEndpoints] = useState<Endpoint[]>([]);
  const [filterMethod, setFilterMethod] = useState("");
  const [filterTag, setFilterTag] = useState("");
  const [loading, setLoading] = useState(false);
  const [discovering, setDiscovering] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Endpoint | null>(null);
  const [inspection, setInspection] = useState<Inspection | null>(null);
  const [inspectError, setInspectError] = useState<string | null>(null);
  const [probing, setProbing] = useState(false);

  async function load() {
    setLoading(true);
    try {
      const res = await api.listEndpoints({ method: filterMethod, tag: filterTag });
      setEndpoints(res);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to load endpoints");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    // Initial data fetch on mount — known false positive for async
    // fetch-on-mount (facebook/react#34905); setState happens in the
    // async continuation, not synchronously in the effect body.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
    // Mount-only: intentionally not re-running when filters change (the
    // "Filter" button re-triggers load() explicitly).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function runDiscover() {
    setDiscovering(true);
    try {
      const res = await api.discover();
      setError(
        res.Errors && res.Errors.length > 0
          ? `Discovered with warnings: ${res.Errors.map((e) => `${e.Provider}: ${e.Message}`).join("; ")}`
          : null
      );
      await load();
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "discover failed");
    } finally {
      setDiscovering(false);
    }
  }

  async function select(ep: Endpoint, live: boolean) {
    setSelected(ep);
    setInspection(null);
    setInspectError(null);
    if (live) setProbing(true);
    try {
      const insp = await api.inspect(ep.Path, { method: ep.Method, live });
      setInspection(insp);
    } catch (e) {
      setInspectError(e instanceof ApiError ? e.message : "inspect failed");
    } finally {
      setProbing(false);
    }
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)_360px] gap-6 h-full">
      <div className="flex flex-col gap-4 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <Input value={filterMethod} onChange={setFilterMethod} placeholder="Method (GET, POST...)" className="w-40" />
          <Input value={filterTag} onChange={setFilterTag} placeholder="Tag" className="w-40" />
          <Button variant="secondary" onClick={load}>
            Filter
          </Button>
          <div className="flex-1" />
          <Button onClick={runDiscover} disabled={discovering}>
            {discovering ? "Discovering..." : "Discover"}
          </Button>
        </div>

        {error && <ErrorBanner message={error} />}

        <Card className="p-0 overflow-hidden">
          {loading ? (
            <EmptyState>Loading...</EmptyState>
          ) : endpoints.length === 0 ? (
            <EmptyState>
              No endpoints in the registry. Click &quot;Discover&quot; or run <code>apilens watch</code>.
            </EmptyState>
          ) : (
            <table className="w-full text-sm">
              <tbody>
                {endpoints.map((ep) => (
                  <tr
                    key={ep.ID}
                    onClick={() => select(ep, false)}
                    className={`cursor-pointer border-b border-neutral-800 last:border-0 hover:bg-neutral-800/50 ${
                      selected?.ID === ep.ID ? "bg-neutral-800" : ""
                    }`}
                  >
                    <td className="px-4 py-2 w-24">
                      <MethodBadge method={ep.Method} />
                    </td>
                    <td className="px-4 py-2 font-mono text-neutral-200">{ep.Path}</td>
                    <td className="px-4 py-2 text-neutral-500 text-xs">{ep.Sources.join(", ")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Card>
      </div>

      <Card className="overflow-auto">
        {!selected ? (
          <EmptyState>Select an endpoint to inspect it.</EmptyState>
        ) : (
          <div className="flex flex-col gap-3 text-sm">
            <div className="flex items-center gap-2">
              <MethodBadge method={selected.Method} />
              <span className="font-mono">{selected.Path}</span>
            </div>
            <div className="text-neutral-500 text-xs">Sources: {selected.Sources.join(", ")}</div>
            {selected.Spec?.Name && <div className="text-neutral-400">Name: {selected.Spec.Name}</div>}

            <div className="flex gap-2">
              <Button variant="secondary" onClick={() => select(selected, true)} disabled={probing}>
                {probing ? "Probing..." : "Live probe"}
              </Button>
              {inspection?.Live && (
                <Button
                  variant="secondary"
                  onClick={() => onInspectLive(inspection.Live!.Display)}
                >
                  Open in Request Builder
                </Button>
              )}
            </div>

            {inspectError && <ErrorBanner message={inspectError} />}

            {inspection?.Live && (
              <div className="border-t border-neutral-800 pt-3">
                <div className="text-xs text-neutral-500 mb-1">Live result</div>
                <div className="flex items-center gap-2">
                  <StatusBadge status={inspection.Live.Response.StatusCode} />
                  <span className="text-neutral-500 text-xs">
                    {(inspection.Live.Timing.Duration / 1e6).toFixed(0)}ms
                  </span>
                </div>
              </div>
            )}
          </div>
        )}
      </Card>
    </div>
  );
}
