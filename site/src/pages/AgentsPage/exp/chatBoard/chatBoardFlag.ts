/**
 * The chat board is an experiment. Everything it owns lives in this folder,
 * production code reaches in only through a handful of hook points, and this
 * flag keeps every one of them inert until the user opts in through the
 * settings toggle, the one hook point that stays live. The flag is
 * frontend-only so the experiment can change without touching the server.
 */
import { useSyncExternalStore } from "react";

const KEY = "agents.exp.chat-board";

export const CHAT_BOARD_PATH = "/agents/board";

// In-tab subscribers. The native "storage" event only fires cross-tab, so
// saving notifies same-tab subscribers directly.
const listeners = new Set<() => void>();

export function getChatBoardEnabled(): boolean {
	return localStorage.getItem(KEY) === "true";
}

export function saveChatBoardEnabled(value: boolean): void {
	localStorage.setItem(KEY, String(value));
	for (const fn of listeners) {
		fn();
	}
}

function subscribe(callback: () => void): () => void {
	listeners.add(callback);
	// A cleared storage area arrives with a null key, so treat that as a
	// change to this flag too.
	const onStorage = (e: StorageEvent) => {
		if (e.key === KEY || e.key === null) {
			callback();
		}
	};
	window.addEventListener("storage", onStorage);
	return () => {
		listeners.delete(callback);
		window.removeEventListener("storage", onStorage);
	};
}

/** The opt-in, live: every consumer re-renders when it is saved. */
export function useChatBoardEnabled(): boolean {
	return useSyncExternalStore(subscribe, getChatBoardEnabled);
}
