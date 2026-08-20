import { useSyncExternalStore } from "react";
import {
	belowLgViewportMediaQuery,
	belowMdViewportMediaQuery,
	isBelowLgViewport,
	isBelowMdViewport,
} from "#/utils/mobile";

const subscribeBelowMdViewport = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(belowMdViewportMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

export const useIsBelowMdViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowMdViewport, isBelowMdViewport);
};

const subscribeBelowLgViewport = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(belowLgViewportMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

/**
 * Returns `true` when the viewport width is below the `lg` Tailwind
 * breakpoint (< 1024 px).
 */
export const useIsBelowLgViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowLgViewport, isBelowLgViewport);
};
