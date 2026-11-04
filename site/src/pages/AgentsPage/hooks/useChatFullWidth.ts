import { useSyncExternalStore } from "react";

/** Holds "true" while the chat transcript uses the full width. */
export const chatFullWidthStorageKey = "agents.chat-full-width";

// In-tab subscribers. The native "storage" event only fires
// cross-tab, so we maintain our own listener set for same-tab
// reactivity when the toggle is flipped in settings.
const listeners = new Set<() => void>();

function subscribe(callback: () => void): () => void {
	listeners.add(callback);

	// Cross-tab changes via the native storage event.
	const onStorage = (e: StorageEvent) => {
		if (e.key === chatFullWidthStorageKey) {
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
	return localStorage.getItem(chatFullWidthStorageKey) === "true";
}

/**
 * Returns the Tailwind max-width class for the chat layout
 * based on whether full-width mode is enabled.
 */
export function chatWidthClass(fullWidth: boolean): string {
	return fullWidth ? "max-w-full" : "max-w-3xl";
}

/**
 * Reactive hook for the chat full-width preference. All
 * consumers re-render when the value changes — no page reload
 * required.
 */
export function useChatFullWidth(): [boolean, (v: boolean) => void] {
	const enabled = useSyncExternalStore(subscribe, getSnapshot);

	const setEnabled = (value: boolean) => {
		localStorage.setItem(chatFullWidthStorageKey, String(value));
		for (const fn of listeners) {
			fn();
		}
	};

	return [enabled, setEnabled];
}
