import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type RefCallback,
	type RefObject,
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

// ===========================================================================
// useStickToBottom — scroll-lock hook
// ===========================================================================

/** Pixel threshold for "near bottom" detection. */
const STICK_TO_BOTTOM_OFFSET_PX = 70;

// ---------------------------------------------------------------------------
// Mutable state (not tied to React render cycle)
// ---------------------------------------------------------------------------

interface InternalState {
	scrollElement: HTMLElement | null;
	contentElement: HTMLElement | null;
	programmaticScrollCount: number;
	resizeDifference: number;
	lastScrollTop: number;
	lastClientHeight: number;
	escapedFromLock: boolean;
	internalIsAtBottom: boolean;
	resizeObserver: ResizeObserver | null;
	viewportObserver: ResizeObserver | null;
	previousContentHeight: number | undefined;
	mouseDown: boolean;
	suppressNextResize: boolean;
	activeTouchCount: number;
	pendingPrepend: { scrollHeight: number } | null;
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

/** Assign scrollTop and bump the programmatic-scroll counter so
 *  the next scroll event is not misread as a user-initiated scroll.
 *  Only bumps the counter when scrollTop actually changes, so no-op
 *  writes don't orphan a counter increment without a matching event. */
function scrollTo(s: InternalState, value: number) {
	if (!s.scrollElement) {
		return;
	}

	const prev = s.scrollElement.scrollTop;
	s.scrollElement.scrollTop = value;
	if (s.scrollElement.scrollTop !== prev) {
		s.programmaticScrollCount++;
	}
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
	/** Tell the hook to skip the next content resize auto-pin. */
	suppressNextResize: () => void;
	/** Capture scrollHeight for prepend restoration. */
	capturePrependSnapshot: () => void;
}

function useStickToBottom(): StickToBottomInstance {
	const [isAtBottom, setIsAtBottom] = useState(true);
	const [nearBottom, setNearBottom] = useState(false);

	const stateRef = useRef<InternalState>({
		scrollElement: null,
		contentElement: null,
		programmaticScrollCount: 0,
		resizeDifference: 0,
		lastScrollTop: 0,
		lastClientHeight: 0,
		escapedFromLock: false,
		internalIsAtBottom: true,
		resizeObserver: null,
		viewportObserver: null,
		previousContentHeight: undefined,
		mouseDown: false,
		suppressNextResize: false,
		activeTouchCount: 0,
		pendingPrepend: null,
	});

	// Sync helpers — keep mutable state and React state in lockstep.
	const syncIsAtBottom = (v: boolean) => {
		stateRef.current.internalIsAtBottom = v;
		setIsAtBottom(v);
	};

	const syncEscapedFromLock = (v: boolean) => {
		stateRef.current.escapedFromLock = v;
	};

	// -----------------------------------------------------------------------
	// scrollToBottom
	// -----------------------------------------------------------------------

	const scrollToBottom = (behavior?: ScrollBehavior) => {
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
			// Don't bump programmaticScrollCount for smooth scroll.
			// Each animation frame naturally reads as a downward
			// scroll (currentScrollTop > lastScrollTop), which
			// correctly clears escapedFromLock.
		}
	};

	const suppressNextResize = () => {
		stateRef.current.suppressNextResize = true;
	};

	const capturePrependSnapshot = () => {
		const s = stateRef.current;
		if (s.scrollElement) {
			s.pendingPrepend = {
				scrollHeight: s.scrollElement.scrollHeight,
			};
		}
	};

	// -----------------------------------------------------------------------
	// Event handlers
	// -----------------------------------------------------------------------

	const handleScroll = useEffectEvent((e: Event) => {
		const s = stateRef.current;
		const { scrollElement } = s;
		if (e.target !== scrollElement || !scrollElement) return;

		const currentScrollTop = scrollElement.scrollTop;

		// Detect viewport-size changes (e.g. Safari PWA toolbar
		// settling, virtual keyboard, safe-area inset shifts).
		// The browser may clamp scrollTop before the
		// ResizeObserver fires, so this scroll event would look
		// like an upward user scroll without this guard.
		const currentClientHeight = scrollElement.clientHeight;
		const viewportChanged = currentClientHeight !== s.lastClientHeight;
		s.lastClientHeight = currentClientHeight;

		// If this event was caused by a programmatic scrollTo,
		// consume the counter and skip escape processing.
		if (s.programmaticScrollCount > 0) {
			s.programmaticScrollCount--;
			s.lastScrollTop = currentScrollTop;
			setNearBottom(isNearBottom(s));
			return;
		}

		const lastST = s.lastScrollTop;
		s.lastScrollTop = currentScrollTop;

		setNearBottom(isNearBottom(s));

		// Synchronous escape logic — must run before any resize
		// handler so they see up-to-date internalIsAtBottom.
		// Skip when a content resize or viewport resize is in
		// progress — the browser may fire scroll events during
		// layout that aren’t user-initiated.
		if (
			s.resizeDifference === 0 &&
			!viewportChanged &&
			s.activeTouchCount === 0
		) {
			if (currentScrollTop < lastST) {
				// If we believe we're at the bottom and the user
				// hasn't escaped via wheel, touch, or scrollbar,
				// this upward movement is browser-initiated (e.g.
				// Safari scroll restoration, focus-driven scroll).
				// Re-pin instead of escaping.
				if (s.internalIsAtBottom && !s.escapedFromLock) {
					scrollTo(s, maxScrollTop(s));
				} else {
					syncEscapedFromLock(true);
					syncIsAtBottom(false);
				}
			} else if (currentScrollTop > lastST) {
				syncEscapedFromLock(false);
			}

			if (!s.escapedFromLock && isNearBottom(s)) {
				syncIsAtBottom(true);
			}
		}
		// Text-selection escape deferred — getSelection() needs
		// post-layout DOM state to reflect the current drag.
		setTimeout(() => {
			if (s.resizeDifference !== 0) return;
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
					}
				}
			}
		}, 1);
	});

	const handleWheel = useEffectEvent((e: WheelEvent) => {
		const s = stateRef.current;

		// Walk up from target to find the nearest scrollable ancestor.
		let el = e.target as HTMLElement | null;
		while (el && el !== s.scrollElement) {
			if (el.scrollHeight > el.clientHeight) {
				const style = getComputedStyle(el);
				if (
					style.overflow === "scroll" ||
					style.overflow === "auto" ||
					style.overflowY === "scroll" ||
					style.overflowY === "auto"
				) {
					break;
				}
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
			// Cancel any in-progress smooth scroll so the animation
			// doesn't override the user's escape intent.
			s.scrollElement.scrollTo({
				top: s.scrollElement.scrollTop,
				behavior: "instant",
			});
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

		// Skip auto-pin while touch contacts are active to
		// prevent mobile URL bar resizes from fighting the
		// user's finger.
		if (s.activeTouchCount > 0) {
			// No auto-pin during active touch.
		} else if (s.pendingPrepend) {
			// Prepend restoration: older messages were just added
			// to the DOM. Adjust scrollTop by the height delta so
			// the user's visual position stays the same.
			const delta =
				s.scrollElement.scrollHeight - s.pendingPrepend.scrollHeight;
			if (delta > 0) {
				const prev = s.scrollElement.scrollTop;
				s.scrollElement.scrollTop = prev + delta;
				if (s.scrollElement.scrollTop !== prev) {
					s.programmaticScrollCount++;
				}
			}
			s.pendingPrepend = null;
		} else if (s.suppressNextResize) {
			s.suppressNextResize = false;
		} else if (difference >= 0) {
			if (previousHeight === undefined) {
				// First observation — jump to bottom instantly.
				if (s.internalIsAtBottom) {
					scrollTo(s, target);
				}
			} else {
				// Check whether we were near the OLD bottom before
				// this resize. We can't rely on internalIsAtBottom
				// alone because scroll events fire during browser
				// layout (before this ResizeObserver callback), and
				// the handler may have disengaged the lock when it
				// saw the scroll position was far from the new,
				// taller bottom.
				const prevMaxScroll = Math.max(
					0,
					previousHeight - s.scrollElement.clientHeight,
				);
				const wasAtBottom =
					s.internalIsAtBottom ||
					(!s.escapedFromLock &&
						s.scrollElement.scrollTop >=
							prevMaxScroll - STICK_TO_BOTTOM_OFFSET_PX);

				if (wasAtBottom) {
					scrollTo(s, target);
					syncIsAtBottom(true);
					syncEscapedFromLock(false);
				}
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

	// When the scroll container's viewport dimensions change (e.g.
	// the top bar gains elements after async data loads), maxScrollTop
	// shifts and we may no longer be at the bottom. Re-pin if locked
	// or physically near the bottom. The near-bottom fallback handles
	// the case where the browser clamped scrollTop before this
	// observer fired, causing the synchronous escape logic in
	// handleScroll to disengage the lock.
	const handleViewportResize = useEffectEvent(() => {
		const s = stateRef.current;
		if (!s.scrollElement) return;
		const maxST = maxScrollTop(s);
		const near = isNearBottom(s);
		if (s.activeTouchCount > 0 || (!s.internalIsAtBottom && !near)) {
			return;
		}

		scrollTo(s, maxST);
		syncIsAtBottom(true);
		syncEscapedFromLock(false);
	});

	const handleTouchStart = useEffectEvent((e: TouchEvent) => {
		stateRef.current.activeTouchCount += Math.max(e.changedTouches.length, 1);
		syncEscapedFromLock(true);
		syncIsAtBottom(false);
	});

	const handleTouchEnd = useEffectEvent((e: TouchEvent) => {
		const s = stateRef.current;
		s.activeTouchCount = Math.max(
			0,
			s.activeTouchCount - Math.max(e.changedTouches.length, 1),
		);
	});

	// -----------------------------------------------------------------------
	// Ref callbacks
	// -----------------------------------------------------------------------

	const handlePointerDown = useEffectEvent((e: PointerEvent) => {
		const s = stateRef.current;
		// e.target === s.scrollElement is only true when clicking
		// the scrollbar track/thumb, not content inside the container.
		if (e.target === s.scrollElement) {
			syncEscapedFromLock(true);
			syncIsAtBottom(false);
		}
	});

	// Ref callbacks must have stable identity — React cycles them
	// on identity change, which leaks event listeners. Store the
	// element in state and let a useEffect manage listeners.
	const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(
		null,
	);

	useEffect(() => {
		const s = stateRef.current;
		s.scrollElement = scrollElement;
		if (!scrollElement) return;

		s.lastClientHeight = scrollElement.clientHeight;
		scrollElement.addEventListener("scroll", handleScroll, { passive: true });
		scrollElement.addEventListener("wheel", handleWheel, { passive: true });
		scrollElement.addEventListener("touchstart", handleTouchStart, {
			passive: true,
		});
		scrollElement.addEventListener("touchend", handleTouchEnd, {
			passive: true,
		});
		scrollElement.addEventListener("touchcancel", handleTouchEnd, {
			passive: true,
		});
		scrollElement.addEventListener("pointerdown", handlePointerDown);

		const vo = new ResizeObserver(handleViewportResize);
		vo.observe(scrollElement);
		s.viewportObserver = vo;

		return () => {
			scrollElement.removeEventListener("touchstart", handleTouchStart);
			scrollElement.removeEventListener("touchend", handleTouchEnd);
			scrollElement.removeEventListener("touchcancel", handleTouchEnd);
			scrollElement.removeEventListener("scroll", handleScroll);
			scrollElement.removeEventListener("wheel", handleWheel);
			scrollElement.removeEventListener("pointerdown", handlePointerDown);
			if (s.viewportObserver) {
				s.viewportObserver.disconnect();
				s.viewportObserver = null;
			}
		};
	}, [scrollElement]);

	const [contentElement, setContentElement] = useState<HTMLDivElement | null>(
		null,
	);

	useEffect(() => {
		const s = stateRef.current;
		s.contentElement = contentElement;
		if (!contentElement) return;

		const ro = new ResizeObserver(handleContentResize);
		ro.observe(contentElement);
		s.resizeObserver = ro;

		return () => {
			ro.disconnect();
			s.resizeObserver = null;
		};
	}, [contentElement]);

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

	// Reset touch counter on tab switch. The browser may not
	// fire touchend/touchcancel when the user switches away
	// mid-gesture, leaving the counter positive and blocking
	// resize observer pins permanently.
	useEffect(() => {
		const s = stateRef.current;
		const handleVisibilityChange = () => {
			if (document.hidden) {
				s.activeTouchCount = 0;
			}
		};
		document.addEventListener("visibilitychange", handleVisibilityChange);
		return () => {
			document.removeEventListener("visibilitychange", handleVisibilityChange);
		};
	}, []);

	// Post-render consistency check. If we believe we're pinned
	// to the bottom but the physical scroll position disagrees,
	// correct it before the browser paints. This catches any
	// race between ResizeObserver callbacks, browser scroll
	// clamping, and React re-renders (e.g. Safari PWA viewport
	// settling after navigation).
	// Intentionally no deps — runs every render as a safety net.
	useLayoutEffect(() => {
		const s = stateRef.current;
		if (!s.scrollElement || !s.internalIsAtBottom) return;
		const target = maxScrollTop(s);
		// 1px tolerance for sub-pixel rounding.
		if (target - s.scrollElement.scrollTop > 1) {
			scrollTo(s, target);
		}
	});

	return {
		scrollRef: setScrollElement,
		contentRef: setContentElement,
		scrollToBottom,
		isAtBottom: isAtBottom || nearBottom,
		suppressNextResize,
		capturePrependSnapshot,
	};
}

// ===========================================================================
// ChatScrollContainer — the scroll-anchored wrapper for the chat transcript
// ===========================================================================

/**
 * Scroll container that keeps the transcript pinned to the bottom using
 * ResizeObserver-driven scroll tracking. Handles:
 * - Stick-to-bottom with automatic re-engagement when content grows.
 * - Loading older message pages via an IntersectionObserver sentinel.
 * - Scroll position restoration when older messages are prepended.
 * - A floating "Scroll to bottom" button when the user scrolls away.
 */
const ChatScrollContainer: FC<{
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: React.ReactNode;
}> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const {
		scrollRef,
		contentRef,
		scrollToBottom,
		isAtBottom,
		capturePrependSnapshot,
	} = useStickToBottom();

	// Merge our callback ref with the external RefObject so both
	// point at the same DOM node, and expose scrollToBottom to the
	// parent via its imperative ref.
	const mergedScrollRef = (el: HTMLDivElement | null) => {
		scrollRef(el);
		scrollContainerRef.current = el;
		scrollToBottomRef.current = el ? () => scrollToBottom("instant") : null;
	};

	// -------------------------------------------------------------------
	// Pagination sentinel (IntersectionObserver)
	// -------------------------------------------------------------------

	const sentinelRef = useRef<HTMLDivElement>(null);
	const observerRef = useRef<IntersectionObserver | null>(null);
	const isFetchingRef = useRef(isFetchingMoreMessages);
	const hasFetchedRef = useRef(false);

	const wasFetchingRef = useRef(false);

	useLayoutEffect(() => {
		const wasFetching = wasFetchingRef.current;
		isFetchingRef.current = isFetchingMoreMessages;
		wasFetchingRef.current = isFetchingMoreMessages;

		if (!wasFetching && isFetchingMoreMessages) {
			hasFetchedRef.current = true;
			capturePrependSnapshot();
		}
		// Restoration happens in handleContentResize (via
		// pendingPrepend) when the DOM actually reflects the
		// prepended content — not here, because the store
		// update may lag behind isFetchingMoreMessages.
	}, [isFetchingMoreMessages, capturePrependSnapshot]);

	useEffect(() => {
		const sentinel = sentinelRef.current;
		const container = scrollContainerRef.current;
		if (!sentinel || !container) return;

		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry.isIntersecting && !isFetchingRef.current) {
					onFetchMoreMessages();
				}
			},
			{
				root: container,
				rootMargin: "600px 0px 0px 0px",
				threshold: 0.01,
			},
		);
		observerRef.current = observer;

		// Defer observation via double-rAF so the initial bottom
		// pin settles before the sentinel can trigger.
		let deferInnerId: number | null = null;
		const deferOuterId = requestAnimationFrame(() => {
			deferInnerId = requestAnimationFrame(() => {
				observer.observe(sentinel);
			});
		});
		return () => {
			cancelAnimationFrame(deferOuterId);
			if (deferInnerId !== null) {
				cancelAnimationFrame(deferInnerId);
			}
			observer.disconnect();
			observerRef.current = null;
		};
	}, [scrollContainerRef, onFetchMoreMessages]);

	// Re-observe the sentinel after a fetch completes so the
	// IntersectionObserver fires again if it stayed visible.
	useEffect(() => {
		if (isFetchingMoreMessages) return;
		if (!hasFetchedRef.current) return;

		const sentinel = sentinelRef.current;
		const observer = observerRef.current;
		if (sentinel && observer) {
			observer.unobserve(sentinel);
			observer.observe(sentinel);
		}
	}, [isFetchingMoreMessages]);

	// -------------------------------------------------------------------
	// Render
	// -------------------------------------------------------------------

	const showButton = !isAtBottom;

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={mergedScrollRef}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [overscroll-behavior:contain] [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div ref={contentRef}>
					{hasMoreMessages && (
						<div ref={sentinelRef} className="h-px shrink-0" />
					)}
					{children}
				</div>
			</div>
			<div className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center overflow-y-auto py-2 [scrollbar-gutter:stable] [scrollbar-width:thin]">
				<Button
					variant="outline"
					size="icon"
					className={cn(
						"rounded-full bg-surface-primary shadow-md transition-all duration-200",
						showButton
							? "pointer-events-auto translate-y-0 opacity-100"
							: "translate-y-2 opacity-0",
					)}
					onClick={() => scrollToBottom()}
					aria-label="Scroll to bottom"
					aria-hidden={!showButton || undefined}
					tabIndex={showButton ? undefined : -1}
				>
					<ArrowDownIcon />
				</Button>
			</div>
		</div>
	);
};

export { ChatScrollContainer };
