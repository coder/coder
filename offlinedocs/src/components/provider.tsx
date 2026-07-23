"use client";
import { RootProvider } from "fumadocs-ui/provider/next";
import type { ReactNode } from "react";

export function Provider({ children }: { children: ReactNode }) {
	// Client-side search: `staticGET` (src/app/api/search/route.ts) emits the
	// Orama search index as a static file at build time, and the `static` client
	// fetches and queries it entirely in the browser, so search works with no
	// server and fully offline.
	return (
		<RootProvider search={{ options: { type: "static" } }}>
			{children}
		</RootProvider>
	);
}
