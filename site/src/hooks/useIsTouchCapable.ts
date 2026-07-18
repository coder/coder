import { useSyncExternalStore } from "react";
import { isTouchCapable, touchCapableMediaQuery } from "#/utils/mobile";

const subscribeTouchCapability = (onStoreChange: () => void) => {
	const mediaQuery = window.matchMedia(touchCapableMediaQuery);
	mediaQuery.addEventListener("change", onStoreChange);
	return () => mediaQuery.removeEventListener("change", onStoreChange);
};

export const useIsTouchCapable = (): boolean => {
	return useSyncExternalStore(subscribeTouchCapability, isTouchCapable);
};
