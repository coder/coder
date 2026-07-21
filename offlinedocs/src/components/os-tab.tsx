"use client";

import { Tabs } from "fumadocs-ui/components/tabs";
import { type ReactNode, useRef } from "react";

// Shared identifier so every OS tab set on the site stays in sync (and syncs
// with itself across client navigations). Must match the value the Fumadocs
// `Tabs` `groupId` store reads and writes.
const OS_GROUP = "os";

// Preference order used when the user agent matches more than one OS. Also the
// set of operating systems OSTab knows how to detect.
const OS_ORDER = ["macOS", "Linux", "Windows"] as const;

// Match the escaping Fumadocs' Tabs applies to a label before comparing it to
// the stored group value (lowercase, first whitespace to a dash).
function escapeValue(value: string): string {
	return value.toLowerCase().replace(/\s/, "-");
}

// Best-effort OS detection from the user agent. Returns the matching label from
// `items` (so a set that omits an OS simply falls through) or undefined.
function detectOS(items: string[]): string | undefined {
	if (typeof navigator === "undefined") return undefined;
	const ua = `${navigator.userAgent} ${navigator.platform ?? ""}`;
	const matches: Record<(typeof OS_ORDER)[number], boolean> = {
		macOS: /Mac/i.test(ua) && !/(iPhone|iPad|iPod)/i.test(ua),
		Linux: /(Linux|X11)/i.test(ua) && !/Android/i.test(ua),
		Windows: /Win/i.test(ua),
	};
	for (const os of OS_ORDER) {
		if (!matches[os]) continue;
		const match = items.find((item) => item.toLowerCase() === os.toLowerCase());
		if (match) return match;
	}
	return undefined;
}

/**
 * OS-aware Tabs.
 *
 * Renders Fumadocs Tabs under a shared `os` group so every OS tab set stays in
 * sync, and, on the first visit with no stored choice, selects the tab matching
 * the reader's operating system.
 *
 * Detection is client-only. Fumadocs' Tabs reads the group store in a mount
 * layout effect, so seeding `sessionStorage` during the first client render
 * (never on the server) sets the initial tab without changing the hydrated
 * markup, so there is no hydration mismatch. A persisted explicit choice
 * (`localStorage`, written by Tabs on click) is left untouched, and if the OS
 * cannot be detected or is not one of the tabs, Tabs keeps its default.
 */
export function OSTab({
	items,
	children,
}: {
	items: string[];
	children: ReactNode;
}) {
	const seeded = useRef(false);
	if (!seeded.current && typeof window !== "undefined") {
		seeded.current = true;
		try {
			const stored =
				sessionStorage.getItem(OS_GROUP) ?? localStorage.getItem(OS_GROUP);
			if (!stored) {
				const os = detectOS(items);
				if (os) sessionStorage.setItem(OS_GROUP, escapeValue(os));
			}
		} catch {
			// sessionStorage/localStorage can throw (privacy mode, sandboxed
			// iframe). Fall back to the default tab.
		}
	}

	return (
		<Tabs items={items} groupId={OS_GROUP} persist>
			{children}
		</Tabs>
	);
}
