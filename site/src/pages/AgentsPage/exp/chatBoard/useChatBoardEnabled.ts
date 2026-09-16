/**
 * The chat board is an experiment. Everything it owns lives in this folder,
 * production code reaches in only through a handful of hook points, and this
 * flag keeps every one of them inert until the user opts in. The flag is
 * frontend-only so the experiment can change without touching the server.
 */
import { useSyncExternalStore } from "react";

const KEY = "agents.exp.chat-board";

export const CHAT_BOARD_PATH = "/agents/board";

// In-tab subscribers. The native "storage" event only fires cross-tab, so
// the settings toggle notifies same-tab consumers through this set.
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

/** Reactive chat board opt-in. Off unless the user enabled it in settings. */
export function useChatBoardEnabled(): [boolean, (v: boolean) => void] {
	const enabled = useSyncExternalStore(subscribe, getSnapshot);

	const setEnabled = (value: boolean) => {
		localStorage.setItem(KEY, String(value));
		for (const fn of listeners) {
			fn();
		}
	};

	return [enabled, setEnabled];
}

/** True while the board is enabled and `pathname` is under its route. */
export function useIsChatBoardRoute(pathname: string): boolean {
	const [enabled] = useChatBoardEnabled();
	return enabled && pathname.startsWith(CHAT_BOARD_PATH);
}
