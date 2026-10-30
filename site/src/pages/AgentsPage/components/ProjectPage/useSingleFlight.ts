import { useRef } from "react";

/**
 * Lets one change save at a time. isPending reaches the controls only after
 * a render, so a fast double click could otherwise submit the same change
 * twice.
 */
export const useSingleFlight = () => {
	const inFlight = useRef(false);
	return {
		/**
		 * Calls `start` unless a change is still saving. Pass the `onSettled`
		 * it receives to the mutation.
		 */
		run: (start: (onSettled: () => void) => void) => {
			if (inFlight.current) {
				return;
			}
			inFlight.current = true;
			start(() => {
				inFlight.current = false;
			});
		},
		isInFlight: () => inFlight.current,
	};
};
