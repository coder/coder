import { type DependencyList, type RefObject, useLayoutEffect } from "react";

/**
 * Keeps a scroll container pinned to the bottom whenever `deps` change and
 * `enabled` is true.
 *
 * It sets `scrollTop` to the full `scrollHeight`, so the container reaches its
 * true bottom regardless of any padding applied to it. react-window's
 * `scrollToItem(..., "end")` only accounts for `itemCount * itemSize` and is
 * unaware of container padding, so it stops short by the padding amount and the
 * last lines can never come into view (coder/coder#25692).
 */
export function useAutoScrollToBottom(
	outerRef: RefObject<HTMLElement | null>,
	enabled: boolean,
	deps: DependencyList,
): void {
	useLayoutEffect(() => {
		if (!enabled) {
			return;
		}
		const el = outerRef.current;
		if (el) {
			el.scrollTop = el.scrollHeight;
		}
	}, [enabled, outerRef, ...deps]);
}
