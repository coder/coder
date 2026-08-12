"use client";

import { AppleLogo, LinuxLogo, WindowsLogo } from "@phosphor-icons/react/ssr";
import {
	TabsContent,
	TabsList,
	TabsTrigger,
} from "fumadocs-ui/components/tabs";
// The unstyled Tabs container is the same component the styled `Tabs` wraps, but
// it accepts a controlled `value`/`onValueChange` (the styled one omits them).
// We need that control to clamp the shared group value (see below).
import { Tabs } from "fumadocs-ui/components/tabs.unstyled";
import {
	Children,
	isValidElement,
	type ReactElement,
	type ReactNode,
	useRef,
	useState,
} from "react";

// Shared identifier so every OS tab set on the site stays in sync (and syncs
// with itself across client navigations). Must match the value the Fumadocs
// `Tabs` `groupId` store reads and writes.
const OS_GROUP = "os";

// Preference order used when the user agent matches more than one OS. Also the
// set of operating systems OSTab knows how to detect.
const OS_ORDER = ["macOS", "Linux", "Windows"] as const;

// Brand glyphs for the OS switcher triggers, keyed by the escaped tab value
// (escapeValue("macOS") === "macos", and so on).
const OS_ICONS: Record<string, ReactNode> = {
	macos: <AppleLogo />,
	linux: <LinuxLogo />,
	windows: <WindowsLogo />,
};

// Container styling copied from Fumadocs' styled `Tabs`; the unstyled container
// we build on omits it. The styled `TabsList`/`TabsTrigger`/`TabsContent` below
// keep their own styling.
const TABS_CLASSNAME =
	"flex flex-col overflow-hidden rounded-xl border bg-fd-secondary my-4";

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
 *
 * OS tab sets offer different subsets of operating systems (some show all
 * three, some only macOS/Windows). The `os` group value is shared across every
 * set, so a value one set stores (e.g. `linux`) is applied to every other set
 * too. Fumadocs only guards that in its styled `Tabs` when `items` is passed,
 * which also renders a plain text tab list and would drop the OS icons. So we
 * build on the unstyled container with a controlled value and clamp updates to
 * the operating systems this set actually offers: an out-of-set value is
 * ignored (this set keeps its default) while the shared choice stays intact for
 * the sets that can honor it.
 */
export function OSTab({
	items,
	children,
}: {
	items: string[];
	children: ReactNode;
}) {
	const values = items.map(escapeValue);
	const [value, setValue] = useState(() => values[0]);

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
		<Tabs
			className={TABS_CLASSNAME}
			groupId={OS_GROUP}
			persist
			value={value}
			onValueChange={(next) => {
				// Only adopt a group value this set can display. The seeded/broadcast
				// value may be an OS this set does not offer; adopting it would select
				// no panel and render an empty box.
				if (values.includes(next)) setValue(next);
			}}
		>
			<TabsList>
				{items.map((os) => {
					const v = escapeValue(os);
					return (
						<TabsTrigger key={v} value={v}>
							{OS_ICONS[v]}
							{os}
						</TabsTrigger>
					);
				})}
			</TabsList>
			{/*
			 * Panels arrive as `<Tab value="macOS">` (Fumadocs' styled `Tab`, which
			 * needs the styled `Tabs` context we are not using). Render them as
			 * `TabsContent` instead - the same element `Tab` produces - so they work
			 * under the unstyled container. Their `value` is escaped to match the
			 * triggers.
			 */}
			{Children.map(children, (child) => {
				if (!isValidElement(child)) return child;
				const panel = child as ReactElement<{
					value?: string;
					children?: ReactNode;
				}>;
				const raw = panel.props.value;
				if (typeof raw !== "string") return child;
				return (
					<TabsContent value={escapeValue(raw)}>
						{panel.props.children}
					</TabsContent>
				);
			})}
		</Tabs>
	);
}
