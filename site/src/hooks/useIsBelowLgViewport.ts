import { useSyncExternalStore } from "react";
import {
	belowLgViewportMediaQuery,
	createMediaQuerySubscribe,
	isBelowLgViewport,
} from "#/utils/mobile";

const subscribeBelowLgViewport = createMediaQuerySubscribe(
	belowLgViewportMediaQuery,
);

export const useIsBelowLgViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowLgViewport, isBelowLgViewport);
};
