import { source } from "@/lib/source";
import { DisplayPreferencesScript, DocsShell, coderFaviconIcons } from "@coder/docs-next";
import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./global.css";

export const metadata: Metadata = {
  title: { template: "%s | Coder Docs", default: "Coder Docs" },
  icons: coderFaviconIcons,
};

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <DisplayPreferencesScript />
      </head>
      <body className="flex min-h-screen flex-col">
        <DocsShell
          source={source}
          providerOptions={{ search: { hybrid: process.env.DOCS_SEMANTIC_SEARCH === "1" } }}
        >
          {children}
        </DocsShell>
      </body>
    </html>
  );
}
