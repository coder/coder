/**
 * The chat board is an experiment. Everything it owns lives in this folder,
 * production code reaches in only through a handful of hook points, and this
 * flag keeps every one of them inert until the deployment enables the
 * chat-board experiment and the user opts in through the settings toggle.
 * The experiment hides the board from deployments that have not asked for
 * it; the opt-in is per browser so trying it affects no one else.
 */
import { useSyncExternalStore } from "react";
import { useDashboard } from "#/modules/dashboard/useDashboard";

const KEY = "agents.exp.chat-board";

export const CHAT_BOARD_PATH = "/agents/board";

// In-tab subscribers. The native "storage" event only fires cross-tab, so
// saving notifies same-tab subscribers directly.
const listeners = new Set<() => void>();

function getOptIn(): boolean {
	return localStorage.getItem(KEY) === "true";
}

export function saveChatBoardOptIn(value: boolean): void {
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

/** This browser's opt-in, live: every consumer re-renders when it is saved. */
export function useChatBoardOptIn(): boolean {
	return useSyncExternalStore(subscribe, getOptIn);
}

/** Whether the deployment offers the board, which shows its settings toggle. */
export function useChatBoardAvailable(): boolean {
	return useDashboard().experiments.includes("chat-board");
}

/**
 * Whether the board is on. The opt-in is kept while the deployment
 * experiment is off, so re-enabling the experiment restores the board.
 */
export function useChatBoardEnabled(): boolean {
	const available = useChatBoardAvailable();
	const optedIn = useChatBoardOptIn();
	return available && optedIn;
}
