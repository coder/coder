import { useSyncExternalStore } from "react";
import { belowMdViewportMediaQuery, isBelowMdViewport } from "#/utils/mobile";

const subscribeBelowMdViewport = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(belowMdViewportMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

export const useIsBelowMdViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowMdViewport, isBelowMdViewport);
};
