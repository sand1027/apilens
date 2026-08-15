"use client";

import { useEffect, useState } from "react";
import { api, ApiError, type Environment } from "@/lib/api";

// Environment switcher tab (plan.md v6). Backed entirely by
// GET /api/environments and POST /api/environments/use — the same
// UseEnv/Environments/CurrentEnv Engine methods `apilens env` uses.
export default function EnvironmentSwitcher() {
  const [items, setItems] = useState<Environment[]>([]);
  const [current, setCurrent] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  async function load() {
    try {
      const res = await api.listEnvironments();
      setItems(res.items ?? []);
      setCurrent(res.current);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to load environments");
    }
  }

  useEffect(() => {
    // Initial data fetch on mount — known false positive for async
    // fetch-on-mount (facebook/react#34905); setState happens in the
    // async continuation, not synchronously in the effect body.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    load();
  }, []);

  async function onChange(name: string) {
    try {
      await api.useEnvironment(name);
      setCurrent(name);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "failed to switch environment");
    }
  }

  return (
    <div className="flex items-center gap-2">
      {error && <span className="text-xs text-red-400">{error}</span>}
      <select
        value={current}
        onChange={(e) => onChange(e.target.value)}
        className="bg-neutral-800 border border-neutral-700 rounded-md px-2 py-1.5 text-sm text-neutral-100 focus:outline-none focus:border-blue-500"
      >
        {items.length === 0 && <option value="">no environments</option>}
        {items.map((env) => (
          <option key={env.Name} value={env.Name}>
            {env.Name}
          </option>
        ))}
      </select>
    </div>
  );
}
