import type { NextConfig } from "next";

// Static export so the Go binary can embed and serve plain files
// (plan.md v6: "web/dashboard/ ... Go serves it. Business logic stays in
// pkg/apilens."). This is a single-page app (see app/page.tsx) — tabs are
// client-side state, not Next.js routes — to avoid any multi-route
// static-export edge cases in this Next.js version.
const nextConfig: NextConfig = {
  output: "export",
  images: {
    unoptimized: true,
  },
};

export default nextConfig;
