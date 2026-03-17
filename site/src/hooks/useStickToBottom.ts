import { useCallback, useEffect, useRef, useState } from "react";

/**
 * Threshold in pixels from the bottom of a flex-col-reverse scroll
 * container. When scrollTop is within this range the user is
 * considered "at the bottom" and auto-scroll stays enabled.
 */
const STICK_THRESHOLD_PX = 48;

interface UseStickToBottomReturn {
	/** Whether the view is currently stuck to the bottom. */
	isStuck: boolean;
	/** Programmatically scroll to the bottom and re-enable stick. */
	scrollToBottom: () => void;
	/**
	 * Callback ref — pass this as the `ref` prop on the scroll
	 * container so the hook can observe it even when the element
	 * mounts after the hook (e.g. loading → loaded transition).
	 */
	scrollRef: (node: HTMLDivElement | null) => void;
}

/**
 * Chat-style "stick to bottom" scroll behaviour for a
 * flex-col-reverse container.
 *
 * In flex-col-reverse, scrollTop === 0 is the visual bottom.
 * Scrolling *up* increases scrollTop. So:
 *   - isStuck  ⟹  scrollTop ≤ threshold
 *   - unstuck  ⟹  scrollTop > threshold (user scrolled up)
 *
 * The scroll container must have `overflow-anchor: none` in CSS
 * to prevent the browser from fighting the user's scroll position
 * during streaming content updates.
 */
export const useStickToBottom = (): UseStickToBottomReturn => {
	const [isStuck, setIsStuck] = useState(true);
	const [scrollEl, setScrollEl] = useState<HTMLDivElement | null>(null);

	// Track the last known scrollTop so we can detect scroll
	// direction inside the passive scroll listener.
	const lastScrollTopRef = useRef(0);

	// Avoid re-subscribing to the scroll event on every render.
	const isStuckRef = useRef(isStuck);
	isStuckRef.current = isStuck;

	const scrollToBottom = useCallback(() => {
		if (!scrollEl) return;
		// Use instant scroll so the UI is consistent — the button
		// disappears and the viewport jumps together.
		scrollEl.scrollTo({ top: 0, behavior: "instant" });
		// Let the scroll handler re-set isStuck on the next event
		// rather than forcing it synchronously here. This avoids
		// a flash where the button hides before the viewport moves.
	}, [scrollEl]);

	useEffect(() => {
		if (!scrollEl) return;
		lastScrollTopRef.current = scrollEl.scrollTop;

		const onScroll = () => {
			const { scrollTop } = scrollEl;
			const nearBottom = scrollTop <= STICK_THRESHOLD_PX;

			// User scrolled up — detach.
			if (scrollTop > lastScrollTopRef.current && !nearBottom) {
				if (isStuckRef.current) {
					setIsStuck(false);
				}
			}

			// User scrolled back to the bottom — re-attach.
			if (nearBottom && !isStuckRef.current) {
				setIsStuck(true);
			}

			lastScrollTopRef.current = scrollTop;
		};

		scrollEl.addEventListener("scroll", onScroll, { passive: true });
		return () => {
			scrollEl.removeEventListener("scroll", onScroll);
		};
	}, [scrollEl]);

	return { isStuck, scrollToBottom, scrollRef: setScrollEl };
};
