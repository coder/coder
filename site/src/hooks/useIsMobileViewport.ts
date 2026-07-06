import { useSyncExternalStore } from "react";
import { isMobileViewport, mobileViewportMediaQuery } from "#/utils/mobile";

const subscribeMobileViewport = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(mobileViewportMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

export const useIsMobileViewport = (): boolean => {
	return useSyncExternalStore(subscribeMobileViewport, isMobileViewport);
};
