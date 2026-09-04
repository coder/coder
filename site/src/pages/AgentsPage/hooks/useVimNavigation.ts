import { useSyncExternalStore } from "react";

export const VIM_NAVIGATION_STORAGE_KEY = "agents.vim-navigation";
const KEY = VIM_NAVIGATION_STORAGE_KEY;

// In-tab subscribers. The native "storage" event only fires
// cross-tab, so we maintain our own listener set for same-tab
// reactivity when the toggle is flipped in settings.
const listeners = new Set<() => void>();

function subscribe(callback: () => void): () => void {
	listeners.add(callback);

	const onStorage = (e: StorageEvent) => {
		if (e.key === KEY) {
			callback();
		}
	};
	window.addEventListener("storage", onStorage);

	return () => {
		listeners.delete(callback);
		window.removeEventListener("storage", onStorage);
	};
}

function getSnapshot(): boolean {
	return localStorage.getItem(KEY) === "true";
}

/**
 * Reactive hook for the vim-style chat navigation preference.
 * When enabled, Cmd/Ctrl+J and Cmd/Ctrl+K move between chats in
 * the sidebar, Cmd/Ctrl+Shift+O starts a new chat, Cmd/Ctrl+Shift+E
 * renames the active chat, and search moves to Cmd/Ctrl+/.
 */
export function useVimNavigation(): [boolean, (v: boolean) => void] {
	const enabled = useSyncExternalStore(subscribe, getSnapshot);

	const setEnabled = (value: boolean) => {
		localStorage.setItem(KEY, String(value));
		for (const fn of listeners) {
			fn();
		}
	};

	return [enabled, setEnabled];
}
