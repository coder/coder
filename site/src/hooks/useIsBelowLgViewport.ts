import { useSyncExternalStore } from "react";
import { belowLgViewportMediaQuery, isBelowLgViewport } from "#/utils/mobile";

const subscribeBelowLgViewport = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(belowLgViewportMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

export const useIsBelowLgViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowLgViewport, isBelowLgViewport);
};
