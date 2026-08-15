"use client";

import { useState } from "react";
import Explorer from "@/components/Explorer";
import RequestBuilder from "@/components/RequestBuilder";
import History from "@/components/History";
import Monitor from "@/components/Monitor";
import TestRunner from "@/components/TestRunner";
import EnvironmentSwitcher from "@/components/EnvironmentSwitcher";
import HitsHUD from "@/components/HitsHUD";

// Tabs are client-side state, not Next.js routes — see next.config.ts for
// why (avoids static-export multi-route edge cases in this Next.js
// version). This also matches the product shape: one local dashboard
// process, one page, several views (plan.md v6 lists Explorer / Request
// builder / History / Runtime monitor / Test suites / Environment
// switcher as views within "apilens ui", not separate apps).
const TABS = [
  { id: "explorer", label: "Explorer" },
  { id: "request", label: "Request Builder" },
  { id: "history", label: "History" },
  { id: "monitor", label: "Runtime Monitor" },
  { id: "tests", label: "Tests" },
] as const;

type TabID = (typeof TABS)[number]["id"];

export default function Page() {
  const [tab, setTab] = useState<TabID>("explorer");
  const [replaySeed, setReplaySeed] = useState<number | null>(null);

  function openInRequestBuilder(displayID: number) {
    setReplaySeed(displayID);
    setTab("request");
  }

  return (
    <div className="flex flex-col h-full min-h-screen">
      <header className="border-b border-neutral-800 px-6 py-4 flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold">ApiLens</h1>
          <p className="text-xs text-neutral-500">Local dashboard — localhost only</p>
        </div>
        <div className="flex items-center gap-3">
          <HitsHUD onOpenInRequestBuilder={openInRequestBuilder} />
          <EnvironmentSwitcher />
        </div>
      </header>

      <nav className="border-b border-neutral-800 px-6 flex gap-1">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`px-3 py-2 text-sm border-b-2 -mb-px transition-colors ${
              tab === t.id
                ? "border-blue-500 text-blue-400"
                : "border-transparent text-neutral-400 hover:text-neutral-200"
            }`}
          >
            {t.label}
          </button>
        ))}
      </nav>

      <main className="flex-1 p-6 overflow-auto">
        {tab === "explorer" && <Explorer onInspectLive={openInRequestBuilder} />}
        {tab === "request" && <RequestBuilder seedDisplayID={replaySeed} />}
        {tab === "history" && <History onOpenInRequestBuilder={openInRequestBuilder} />}
        {tab === "monitor" && <Monitor onOpenInRequestBuilder={openInRequestBuilder} />}
        {tab === "tests" && <TestRunner />}
      </main>
    </div>
  );
}
