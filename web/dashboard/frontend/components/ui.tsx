// Small shared presentational primitives used across tabs. No API calls
// or business logic here — purely styling helpers to keep the tab
// components focused on data flow.
import type { ReactNode } from "react";

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`bg-neutral-900 border border-neutral-800 rounded-lg p-4 ${className}`}>
      {children}
    </div>
  );
}

export function Button({
  children,
  onClick,
  disabled,
  variant = "primary",
  type = "button",
}: {
  children: ReactNode;
  onClick?: () => void;
  disabled?: boolean;
  variant?: "primary" | "secondary" | "danger";
  type?: "button" | "submit";
}) {
  const styles = {
    primary: "bg-blue-600 hover:bg-blue-500 text-white",
    secondary: "bg-neutral-800 hover:bg-neutral-700 text-neutral-100",
    danger: "bg-red-700 hover:bg-red-600 text-white",
  };
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className={`px-3 py-1.5 text-sm rounded-md font-medium disabled:opacity-40 disabled:cursor-not-allowed transition-colors ${styles[variant]}`}
    >
      {children}
    </button>
  );
}

export function Input({
  value,
  onChange,
  placeholder,
  className = "",
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  className?: string;
}) {
  return (
    <input
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className={`bg-neutral-800 border border-neutral-700 rounded-md px-3 py-1.5 text-sm text-neutral-100 placeholder:text-neutral-500 focus:outline-none focus:border-blue-500 ${className}`}
    />
  );
}

export function MethodBadge({ method }: { method: string }) {
  const colors: Record<string, string> = {
    GET: "bg-blue-950 text-blue-300 border-blue-800",
    POST: "bg-green-950 text-green-300 border-green-800",
    QUERY: "bg-blue-950 text-blue-300 border-blue-800",
    MUTATION: "bg-green-950 text-green-300 border-green-800",
    SUBSCRIPTION: "bg-purple-950 text-purple-300 border-purple-800",
    PUT: "bg-yellow-950 text-yellow-300 border-yellow-800",
    PATCH: "bg-yellow-950 text-yellow-300 border-yellow-800",
    DELETE: "bg-red-950 text-red-300 border-red-800",
  };
  return (
    <span
      className={`inline-block px-1.5 py-0.5 text-xs font-mono font-semibold rounded border ${
        colors[method] ?? "bg-neutral-800 text-neutral-300 border-neutral-700"
      }`}
    >
      {method}
    </span>
  );
}

export function StatusBadge({ status }: { status: number }) {
  let color = "text-neutral-400";
  if (status >= 200 && status < 300) color = "text-green-400";
  else if (status >= 400 && status < 500) color = "text-yellow-400";
  else if (status >= 500) color = "text-red-400";
  return <span className={`font-mono text-sm ${color}`}>{status || "—"}</span>;
}

export function EmptyState({ children }: { children: ReactNode }) {
  return <div className="text-sm text-neutral-500 py-8 text-center">{children}</div>;
}

export function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="bg-red-950 border border-red-800 text-red-300 text-sm rounded-md px-3 py-2">
      {message}
    </div>
  );
}
