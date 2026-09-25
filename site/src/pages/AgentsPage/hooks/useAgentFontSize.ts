import { useEffect, useSyncExternalStore } from "react";

export type AgentFontSize = "13" | "14";

export const agentFontSizeStorageKey = "agents.font-size";

const agentFontSizeProperty = "--agent-font-size";

// The native "storage" event only fires cross-tab, so same-tab
// consumers (the settings page and the page shell) share this set.
const listeners = new Set<() => void>();

function subscribe(callback: () => void): () => void {
	listeners.add(callback);
	const onStorage = (e: StorageEvent) => {
		if (e.key === agentFontSizeStorageKey) {
			callback();
		}
	};
	window.addEventListener("storage", onStorage);
	return () => {
		listeners.delete(callback);
		window.removeEventListener("storage", onStorage);
	};
}

function getSnapshot(): AgentFontSize {
	return localStorage.getItem(agentFontSizeStorageKey) === "13" ? "13" : "14";
}

/**
 * Reactive hook for the agents UI base font size, which drives every
 * `text-(length:--agent-font-size)` element. Defaults to 14px; `text-xs` text is unaffected.
 */
export function useAgentFontSize(): [
	AgentFontSize,
	(size: AgentFontSize) => void,
] {
	const size = useSyncExternalStore(subscribe, getSnapshot);

	const setSize = (value: AgentFontSize) => {
		localStorage.setItem(agentFontSizeStorageKey, value);
		for (const fn of listeners) {
			fn();
		}
	};

	return [size, setSize];
}

/**
 * Applies the font size preference to the document element so portaled
 * menus, dialogs, and tooltips pick it up too. Call from each agents
 * page shell.
 */
export function useApplyAgentFontSize(): void {
	const [size] = useAgentFontSize();
	useEffect(() => {
		const root = document.documentElement;
		root.style.setProperty(agentFontSizeProperty, `${size}px`);
		return () => {
			root.style.removeProperty(agentFontSizeProperty);
		};
	}, [size]);
}
