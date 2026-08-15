"use client";

import { useState } from "react";
import { api, ApiError, type Report, type TestResult } from "@/lib/api";
import { Button, Card, EmptyState, ErrorBanner, Input } from "@/components/ui";

// Test suites and results tab (plan.md v6): POST /api/run, the same
// filter fields and Report shape as `apilens run` / `apilens test`.
export default function TestRunner() {
  const [tag, setTag] = useState("");
  const [ref, setRef] = useState("");
  const [failFast, setFailFast] = useState(false);
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);

  async function run() {
    setRunning(true);
    setError(null);
    try {
      const res = await api.run({ ref: ref || undefined, tag: tag || undefined, failFast });
      setReport(res);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "run failed");
      setReport(null);
    } finally {
      setRunning(false);
    }
  }

  return (
    <div className="flex flex-col gap-4 max-w-4xl">
      <Card className="flex items-center gap-3 flex-wrap">
        <Input value={ref} onChange={setRef} placeholder="Filter: path, name, or file" className="w-64" />
        <Input value={tag} onChange={setTag} placeholder="Tag" className="w-40" />
        <label className="flex items-center gap-1.5 text-sm text-neutral-400">
          <input type="checkbox" checked={failFast} onChange={(e) => setFailFast(e.target.checked)} />
          fail fast
        </label>
        <div className="flex-1" />
        <Button onClick={run} disabled={running}>
          {running ? "Running..." : "Run"}
        </Button>
      </Card>

      {error && <ErrorBanner message={error} />}

      {report && (
        <Card>
          <div className="flex items-center gap-4 mb-4 text-sm">
            <Stat label="Tests" value={report.Counts.tests} />
            <Stat label="Passed" value={report.Counts.passed} color="text-green-400" />
            <Stat label="Failed" value={report.Counts.failed} color="text-red-400" />
            <Stat label="Errored" value={report.Counts.errored} color="text-yellow-400" />
            <Stat label="Skipped" value={report.Counts.skipped} color="text-neutral-500" />
            <div className="flex-1" />
            <span className="text-neutral-500">{report.DurationMS}ms</span>
          </div>

          {(!report.Results || report.Results.length === 0) ? (
            <EmptyState>No tests matched.</EmptyState>
          ) : (
            <div className="flex flex-col divide-y divide-neutral-800">
              {report.Results.map((r, i) => <ResultRow key={i} result={r} />)}
            </div>
          )}
        </Card>
      )}
    </div>
  );
}

function Stat({ label, value, color = "text-neutral-200" }: { label: string; value: number; color?: string }) {
  return (
    <div className="flex flex-col">
      <span className="text-xs text-neutral-500">{label}</span>
      <span className={`text-lg font-semibold ${color}`}>{value}</span>
    </div>
  );
}

function ResultRow({ result }: { result: TestResult }) {
  const mark = { passed: "✓", failed: "✗", errored: "✗", skipped: "○" }[result.Status];
  const color = {
    passed: "text-green-400",
    failed: "text-red-400",
    errored: "text-red-400",
    skipped: "text-neutral-500",
  }[result.Status];
  return (
    <div className="py-2 text-sm">
      <div className="flex items-center gap-2">
        <span className={color}>{mark}</span>
        <span className="font-mono">{result.Method}</span>
        <span className="font-mono text-neutral-400 truncate">{result.URL}</span>
        <div className="flex-1" />
        <span className="text-neutral-500 text-xs">{result.HTTPStatus || ""}</span>
        <span className="text-neutral-500 text-xs">{result.DurationMS}ms</span>
      </div>
      {result.Status === "failed" &&
        result.Assertions?.filter((a) => !a.Passed).map((a, i) => (
          <div key={i} className="ml-6 text-xs text-red-400/80 font-mono">
            {a.Kind}: expected {a.Expected}, got {a.Actual}
          </div>
        ))}
      {result.Status === "errored" && result.Error && (
        <div className="ml-6 text-xs text-red-400/80">{result.Error}</div>
      )}
    </div>
  );
}
