import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "ApiLens Dashboard",
  description: "Local API discovery, testing, and traffic monitor.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className="h-full antialiased">
      <body className="min-h-full flex flex-col bg-neutral-950 text-neutral-100">
        {children}
      </body>
    </html>
  );
}
