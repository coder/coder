import { useCallback, useSyncExternalStore } from "react";

/**
 * Subscribes to a CSS media query and returns whether it currently
 * matches, re-rendering on change. Pass a shared query constant from
 * `utils/mobile.ts` so breakpoints stay aligned with Tailwind
 * utilities.
 */
export const useMediaQuery = (query: string): boolean => {
	const subscribe = useCallback(
		(onStoreChange: () => void) => {
			const mediaQuery = window.matchMedia(query);
			mediaQuery.addEventListener("change", onStoreChange);
			return () => mediaQuery.removeEventListener("change", onStoreChange);
		},
		[query],
	);
	return useSyncExternalStore(
		subscribe,
		() => window.matchMedia(query).matches,
	);
};
