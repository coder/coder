import { type RefCallback, useEffect, useRef, useState } from "react";
import { useEffectEvent } from "#/hooks/hookPolyfills";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Pixel threshold for "near bottom" detection. */
const STICK_TO_BOTTOM_OFFSET_PX = 70;

// ---------------------------------------------------------------------------
// Mutable state (not tied to React render cycle)
// ---------------------------------------------------------------------------

interface InternalState {
	scrollElement: HTMLElement | null;
	contentElement: HTMLElement | null;
	ignoreScrollToTop: number | undefined;
	resizeDifference: number;
	lastScrollTop: number;
	escapedFromLock: boolean;
	internalIsAtBottom: boolean;
	resizeObserver: ResizeObserver | null;
	previousContentHeight: number | undefined;
	mouseDown: boolean;
}

/** The maximum scrollable offset for the container. */
function maxScrollTop(s: InternalState): number {
	if (!s.scrollElement) {
		return 0;
	}

	return Math.max(
		0,
		s.scrollElement.scrollHeight - s.scrollElement.clientHeight,
	);
}

/** Whether the scroll position is within the stick-to-bottom threshold. */
function isNearBottom(s: InternalState): boolean {
	if (!s.scrollElement) {
		return false;
	}

	const distance = maxScrollTop(s) - s.scrollElement.scrollTop;

	return distance <= STICK_TO_BOTTOM_OFFSET_PX;
}

/** Assign scrollTop and record the value so the scroll handler can
 *  distinguish this programmatic write from a user-initiated scroll. */
function scrollTo(s: InternalState, value: number) {
	if (!s.scrollElement) {
		return;
	}

	s.scrollElement.scrollTop = value;
	s.ignoreScrollToTop = s.scrollElement.scrollTop;
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

interface StickToBottomInstance {
	scrollRef: RefCallback<HTMLDivElement>;
	contentRef: RefCallback<HTMLDivElement>;
	/** Scroll to the bottom. Pass `"instant"` to jump; omit for smooth. */
	scrollToBottom: (behavior?: ScrollBehavior) => void;
	/** True when the view is locked to the bottom or physically near it. */
	isAtBottom: boolean;
}

function useStickToBottom(): StickToBottomInstance {
	const [isAtBottom, setIsAtBottom] = useState(true);
	const [nearBottom, setNearBottom] = useState(false);

	const stateRef = useRef<InternalState>({
		scrollElement: null,
		contentElement: null,
		ignoreScrollToTop: undefined,
		resizeDifference: 0,
		lastScrollTop: 0,
		escapedFromLock: false,
		internalIsAtBottom: true,
		resizeObserver: null,
		previousContentHeight: undefined,
		mouseDown: false,
	});

	// Sync helpers — keep mutable state and React state in lockstep.
	const syncIsAtBottom = useEffectEvent((v: boolean) => {
		stateRef.current.internalIsAtBottom = v;
		setIsAtBottom(v);
	});

	const syncEscapedFromLock = useEffectEvent((v: boolean) => {
		stateRef.current.escapedFromLock = v;
	});

	// -----------------------------------------------------------------------
	// scrollToBottom
	// -----------------------------------------------------------------------

	const scrollToBottom = useEffectEvent((behavior?: ScrollBehavior) => {
		const s = stateRef.current;
		if (!s.scrollElement) return;

		syncIsAtBottom(true);
		syncEscapedFromLock(false);

		const top = maxScrollTop(s);
		if (behavior === "instant") {
			scrollTo(s, top);
		} else {
			s.scrollElement.scrollTo({
				top,
				behavior: behavior ?? "smooth",
			});
			// Record the target so the scroll handler doesn't
			// interpret the smooth-scroll frames as user scrolls.
			s.ignoreScrollToTop = top;
		}
	});

	// -----------------------------------------------------------------------
	// Event handlers
	// -----------------------------------------------------------------------

	const handleScroll = useEffectEvent((e: Event) => {
		const s = stateRef.current;
		const { scrollElement } = s;
		if (e.target !== scrollElement || !scrollElement) return;

		const currentScrollTop = scrollElement.scrollTop;
		let lastST = s.lastScrollTop;

		if (
			s.ignoreScrollToTop !== undefined &&
			s.ignoreScrollToTop > currentScrollTop
		) {
			lastST = s.ignoreScrollToTop;
		}
		s.ignoreScrollToTop = undefined;
		s.lastScrollTop = currentScrollTop;

		setNearBottom(isNearBottom(s));

		setTimeout(() => {
			if (s.resizeDifference !== 0) return;

			// If the user is selecting text inside the scroll
			// container, treat it as an escape so the selection
			// isn't fought by auto-scroll.
			if (s.mouseDown && s.scrollElement) {
				const sel = window.getSelection();
				if (sel && sel.rangeCount > 0) {
					const ancestor = sel.getRangeAt(0).commonAncestorContainer;
					const el =
						ancestor instanceof HTMLElement ? ancestor : ancestor.parentElement;
					if (
						el &&
						(s.scrollElement.contains(el) || el.contains(s.scrollElement))
					) {
						syncEscapedFromLock(true);
						syncIsAtBottom(false);
						return;
					}
				}
			}

			if (currentScrollTop < lastST) {
				syncEscapedFromLock(true);
				syncIsAtBottom(false);
			} else if (currentScrollTop > lastST) {
				syncEscapedFromLock(false);
			}

			if (!s.escapedFromLock && isNearBottom(s)) {
				syncIsAtBottom(true);
			}
		}, 1);
	});

	const handleWheel = useEffectEvent((e: WheelEvent) => {
		const s = stateRef.current;

		// Walk up from target to find the nearest scrollable ancestor.
		let el = e.target as HTMLElement | null;
		while (el && el !== s.scrollElement) {
			const style = getComputedStyle(el);
			if (
				style.overflow === "scroll" ||
				style.overflow === "auto" ||
				style.overflowY === "scroll" ||
				style.overflowY === "auto"
			) {
				break;
			}
			el = el.parentElement;
		}

		if (
			el === s.scrollElement &&
			e.deltaY < 0 &&
			s.scrollElement &&
			s.scrollElement.scrollHeight > s.scrollElement.clientHeight
		) {
			syncEscapedFromLock(true);
			syncIsAtBottom(false);
		}
	});

	const handleContentResize = useEffectEvent(() => {
		const s = stateRef.current;
		if (!s.contentElement || !s.scrollElement) return;

		const currentHeight = s.contentElement.getBoundingClientRect().height;
		const previousHeight = s.previousContentHeight;
		const difference =
			previousHeight !== undefined ? currentHeight - previousHeight : 0;

		s.resizeDifference = difference;

		// Clamp browser overscroll.
		const target = maxScrollTop(s);
		if (s.scrollElement.scrollTop > target) {
			scrollTo(s, target);
		}

		setNearBottom(isNearBottom(s));

		if (difference >= 0) {
			if (previousHeight === undefined) {
				// First observation — jump to bottom instantly.
				if (s.internalIsAtBottom) {
					scrollTo(s, target);
				}
			} else if (s.internalIsAtBottom) {
				// Content grew while locked — follow instantly.
				scrollTo(s, target);
			}
		} else if (isNearBottom(s)) {
			// Content shrank and we ended up near bottom — re-engage.
			syncEscapedFromLock(false);
			syncIsAtBottom(true);
		}

		s.previousContentHeight = currentHeight;

		// Clear after rAF + setTimeout(1) so the scroll handler has
		// a chance to see the resize flag before it resets.
		const captured = s.resizeDifference;
		requestAnimationFrame(() => {
			setTimeout(() => {
				if (s.resizeDifference === captured) {
					s.resizeDifference = 0;
				}
			}, 1);
		});
	});

	// -----------------------------------------------------------------------
	// Ref callbacks
	// -----------------------------------------------------------------------

	const scrollRef = useEffectEvent((el: HTMLDivElement | null) => {
		const s = stateRef.current;
		const prev = s.scrollElement;

		if (prev) {
			prev.removeEventListener("scroll", handleScroll);
			prev.removeEventListener("wheel", handleWheel);
		}

		s.scrollElement = el;

		if (el) {
			el.addEventListener("scroll", handleScroll, { passive: true });
			el.addEventListener("wheel", handleWheel, { passive: true });
		}
	});

	const contentRef = useEffectEvent((el: HTMLDivElement | null) => {
		const s = stateRef.current;

		if (s.resizeObserver) {
			s.resizeObserver.disconnect();
			s.resizeObserver = null;
		}

		s.contentElement = el;

		if (el) {
			const ro = new ResizeObserver(handleContentResize);
			ro.observe(el);
			s.resizeObserver = ro;
		}
	});

	// -----------------------------------------------------------------------
	// Mouse tracking (instance-scoped)
	// -----------------------------------------------------------------------

	useEffect(() => {
		const s = stateRef.current;
		const onDown = () => {
			s.mouseDown = true;
		};
		const onUp = () => {
			s.mouseDown = false;
		};
		document.addEventListener("mousedown", onDown);
		document.addEventListener("mouseup", onUp);
		document.addEventListener("click", onUp);
		return () => {
			document.removeEventListener("mousedown", onDown);
			document.removeEventListener("mouseup", onUp);
			document.removeEventListener("click", onUp);
		};
	}, []);

	// Cleanup on unmount.
	useEffect(() => {
		return () => {
			const s = stateRef.current;
			if (s.scrollElement) {
				s.scrollElement.removeEventListener("scroll", handleScroll);
				s.scrollElement.removeEventListener("wheel", handleWheel);
			}
			if (s.resizeObserver) {
				s.resizeObserver.disconnect();
			}
		};
	}, [handleScroll, handleWheel]);

	return {
		scrollRef,
		contentRef,
		scrollToBottom,
		isAtBottom: isAtBottom || nearBottom,
	};
}

export { useStickToBottom };
