import { useSyncExternalStore } from "react";

type PreferredColorScheme = "dark" | "light";

const defaultPreferredColorScheme: PreferredColorScheme = "dark";

const getColorSchemeQuery = () => {
	if (typeof window === "undefined") {
		return undefined;
	}
	return window.matchMedia?.("(prefers-color-scheme: light)");
};

const getPreferredColorScheme = (): PreferredColorScheme => {
	const query = getColorSchemeQuery();
	if (!query) {
		// Match the server snapshot so hydration starts from one stable
		// scheme before the browser media query becomes available.
		return defaultPreferredColorScheme;
	}
	return query.matches ? "light" : "dark";
};

const subscribePreferredColorScheme = (onStoreChange: () => void) => {
	const query = getColorSchemeQuery();
	if (!query) {
		return () => {
			// No listener was registered when matchMedia is unavailable.
		};
	}
	query.addEventListener?.("change", onStoreChange);
	return () => {
		query.removeEventListener?.("change", onStoreChange);
	};
};

export const usePreferredColorScheme = (): PreferredColorScheme => {
	return useSyncExternalStore(
		subscribePreferredColorScheme,
		getPreferredColorScheme,
		getPreferredColorScheme,
	);
};
