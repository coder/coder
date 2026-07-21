"use client";
import { RootProvider } from "fumadocs-ui/provider/next";
import type { ReactNode } from "react";

export function Provider({ children }: { children: ReactNode }) {
	// Search is disabled: the bundle is a static export with no server route to
	// back a search index, and it is meant to work fully offline.
	return <RootProvider search={{ enabled: false }}>{children}</RootProvider>;
}
