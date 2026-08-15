"use client";

import { useEffect, useState } from "react";
import { api, ApiError, type Exchange } from "@/lib/api";
import { Button, Card, ErrorBanner, Input } from "@/components/ui";
import { ExchangeDetail } from "@/components/History";

// Request Builder tab (plan.md v6): loads a captured exchange by display
// ID and replays it with overrides — POST /api/replay/{id}, identical to
// `apilens replay <id> --method --url --header --unset --query`. Can also
// write the result as a saved test via POST /api/generate/{id}
// (`apilens generate <id>`).
export default function RequestBuilder({ seedDisplayID }: { seedDisplayID: number | null }) {
  const [idInput, setIdInput] = useState(seedDisplayID != null ? String(seedDisplayID) : "");
  const [method, setMethod] = useState("");
  const [url, setUrl] = useState("");
  const [headerName, setHeaderName] = useState("");
  const [headerValue, setHeaderValue] = useState("");
  const [headers, setHeaders] = useState<Record<string, string>>({});
  const [unsetName, setUnsetName] = useState("");
  const [unset, setUnset] = useState<string[]>([]);
  const [result, setResult] = useState<Exchange | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generatedPath, setGeneratedPath] = useState<string | null>(null);

  useEffect(() => {
    // Syncing local input state from a prop change (the "seed" ID coming
    // from another tab's "Open in Request Builder" action) is exactly the
    // pattern React's own docs list as a valid effect use, but the lint
    // rule can't tell that apart from an accidental cascade — see
    // https://react.dev/learn/you-might-not-need-an-effect#adjusting-some-state-when-a-prop-changes.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    if (seedDisplayID != null) setIdInput(String(seedDisplayID));
  }, [seedDisplayID]);

  function addHeader() {
    if (!headerName) return;
    setHeaders((h) => ({ ...h, [headerName]: headerValue }));
    setHeaderName("");
    setHeaderValue("");
  }

  function addUnset() {
    if (!unsetName) return;
    setUnset((u) => [...u, unsetName]);
    setUnsetName("");
  }

  async function doReplay() {
    const id = parseInt(idInput, 10);
    if (Number.isNaN(id)) {
      setError("enter a valid history id");
      return;
    }
    setBusy(true);
    setGeneratedPath(null);
    try {
      const ex = await api.replay(id, {
        method: method || undefined,
        url: url || undefined,
        headers: Object.keys(headers).length ? headers : undefined,
        unset: unset.length ? unset : undefined,
      });
      setResult(ex);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "replay failed");
    } finally {
      setBusy(false);
    }
  }

  async function doGenerate() {
    if (!result) return;
    setBusy(true);
    try {
      const gen = await api.generate(result.Display, {});
      setGeneratedPath(gen.Path);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "generate failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid grid-cols-[420px_minmax(0,1fr)] gap-6 h-full">
      <Card className="flex flex-col gap-4">
        <div>
          <label className="text-xs text-neutral-500 block mb-1">History ID to replay</label>
          <Input value={idInput} onChange={setIdInput} placeholder="e.g. 1" />
        </div>

        <div className="grid grid-cols-2 gap-2">
          <div>
            <label className="text-xs text-neutral-500 block mb-1">Method override</label>
            <Input value={method} onChange={setMethod} placeholder="(unchanged)" />
          </div>
          <div>
            <label className="text-xs text-neutral-500 block mb-1">URL override</label>
            <Input value={url} onChange={setUrl} placeholder="(unchanged)" />
          </div>
        </div>

        <div>
          <label className="text-xs text-neutral-500 block mb-1">Set/replace header</label>
          <div className="flex gap-2">
            <Input value={headerName} onChange={setHeaderName} placeholder="Name" className="flex-1" />
            <Input value={headerValue} onChange={setHeaderValue} placeholder="Value" className="flex-1" />
            <Button variant="secondary" onClick={addHeader}>
              Add
            </Button>
          </div>
          {Object.entries(headers).length > 0 && (
            <div className="mt-2 text-xs font-mono space-y-1">
              {Object.entries(headers).map(([k, v]) => (
                <div key={k} className="flex justify-between">
                  <span>
                    {k}: {v}
                  </span>
                  <button
                    onClick={() =>
                      setHeaders((h) => {
                        const rest = { ...h };
                        delete rest[k];
                        return rest;
                      })
                    }
                    className="text-neutral-500 hover:text-red-400"
                  >
                    remove
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        <div>
          <label className="text-xs text-neutral-500 block mb-1">Unset header</label>
          <div className="flex gap-2">
            <Input value={unsetName} onChange={setUnsetName} placeholder="Name" className="flex-1" />
            <Button variant="secondary" onClick={addUnset}>
              Add
            </Button>
          </div>
          {unset.length > 0 && (
            <div className="mt-2 text-xs font-mono space-y-1">
              {unset.map((name, i) => (
                <div key={`${name}-${i}`} className="flex justify-between">
                  <span>{name}</span>
                  <button
                    onClick={() => setUnset((u) => u.filter((_, idx) => idx !== i))}
                    className="text-neutral-500 hover:text-red-400"
                  >
                    remove
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {error && <ErrorBanner message={error} />}

        <div className="flex gap-2 mt-auto pt-2 border-t border-neutral-800">
          <Button onClick={doReplay} disabled={busy}>
            {busy ? "Working..." : "Replay"}
          </Button>
          <Button variant="secondary" onClick={doGenerate} disabled={busy || !result}>
            Save as test
          </Button>
        </div>
        {generatedPath && (
          <div className="text-xs text-green-400">Wrote {generatedPath}</div>
        )}
      </Card>

      <Card className="overflow-auto">
        {!result ? (
          <div className="text-sm text-neutral-500 py-8 text-center">
            Enter a history ID and replay to see the result here.
          </div>
        ) : (
          <ExchangeDetail exchange={result} />
        )}
      </Card>
    </div>
  );
}
