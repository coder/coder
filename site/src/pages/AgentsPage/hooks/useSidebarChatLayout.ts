import { useSyncExternalStore } from "react";

export type SidebarChatLayout = "two_line" | "one_line";

export const sidebarChatLayoutStorageKey = "agents.sidebar-chat-layout";

// The native "storage" event only fires cross-tab, so same-tab
// consumers (the settings page and the sidebar) share this set.
const listeners = new Set<() => void>();

function subscribe(callback: () => void): () => void {
	listeners.add(callback);
	const onStorage = (e: StorageEvent) => {
		if (e.key === sidebarChatLayoutStorageKey) {
			callback();
		}
	};
	window.addEventListener("storage", onStorage);
	return () => {
		listeners.delete(callback);
		window.removeEventListener("storage", onStorage);
	};
}

function getSnapshot(): SidebarChatLayout {
	return localStorage.getItem(sidebarChatLayoutStorageKey) === "one_line"
		? "one_line"
		: "two_line";
}

/**
 * Reactive hook for how chats render in the agents sidebar: two lines
 * (title plus PR and summary details) or one line (title only).
 * Defaults to two lines.
 */
export function useSidebarChatLayout(): [
	SidebarChatLayout,
	(layout: SidebarChatLayout) => void,
] {
	const layout = useSyncExternalStore(subscribe, getSnapshot);

	const setLayout = (value: SidebarChatLayout) => {
		localStorage.setItem(sidebarChatLayoutStorageKey, value);
		for (const fn of listeners) {
			fn();
		}
	};

	return [layout, setLayout];
}
