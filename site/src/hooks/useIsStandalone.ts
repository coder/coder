import { useSyncExternalStore } from "react";

const standaloneQuery =
	typeof window !== "undefined"
		? window.matchMedia("(display-mode: standalone)")
		: null;

function getSnapshot(): boolean {
	if (typeof window === "undefined") {
		return false;
	}
	// iOS Safari does not support the display-mode media query but
	// exposes a proprietary `standalone` property on the navigator.
	if ((navigator as unknown as Record<string, unknown>).standalone === true) {
		return true;
	}
	return standaloneQuery?.matches ?? false;
}

function getServerSnapshot(): boolean {
	return false;
}

function subscribe(callback: () => void): () => void {
	standaloneQuery?.addEventListener("change", callback);
	return () => {
		standaloneQuery?.removeEventListener("change", callback);
	};
}

/**
 * Detects whether the app is running as an installed PWA / standalone
 * web app. Handles both the standard `display-mode: standalone` media
 * query and the iOS Safari `navigator.standalone` property.
 */
export function useIsStandalone(): boolean {
	return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}
