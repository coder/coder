import { useSyncExternalStore } from "react";
import {
	belowMdViewportMediaQuery,
	createMediaQuerySubscribe,
	isBelowMdViewport,
} from "#/utils/mobile";

const subscribeBelowMdViewport = createMediaQuerySubscribe(
	belowMdViewportMediaQuery,
);

export const useIsBelowMdViewport = (): boolean => {
	return useSyncExternalStore(subscribeBelowMdViewport, isBelowMdViewport);
};
