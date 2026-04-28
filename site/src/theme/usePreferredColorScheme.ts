import { useSyncExternalStore } from "react";

type PreferredColorScheme = "dark" | "light";

const getColorSchemeQuery = () => {
	if (typeof window === "undefined") {
		return undefined;
	}
	return window.matchMedia?.("(prefers-color-scheme: light)");
};

const getPreferredColorScheme = (): PreferredColorScheme => {
	return getColorSchemeQuery()?.matches ? "light" : "dark";
};

const subscribePreferredColorScheme = (onStoreChange: () => void) => {
	const query = getColorSchemeQuery();
	if (!query) {
		return () => undefined;
	}
	query.addEventListener?.("change", onStoreChange);
	return () => query.removeEventListener?.("change", onStoreChange);
};

export const usePreferredColorScheme = (): PreferredColorScheme => {
	return useSyncExternalStore(
		subscribePreferredColorScheme,
		getPreferredColorScheme,
		getPreferredColorScheme,
	);
};
