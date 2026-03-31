import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type RefCallback,
	type RefObject,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import { useEffectEvent } from "#/hooks/hookPolyfills";
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
	ignoreScrollToTop: number | undefined;
	resizeDifference: number;
	lastScrollTop: number;
	escapedFromLock: boolean;
	internalIsAtBottom: boolean;
	resizeObserver: ResizeObserver | null;
	viewportObserver: ResizeObserver | null;
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
		viewportObserver: null,
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
					s.scrollElement.scrollTop >=
						prevMaxScroll - STICK_TO_BOTTOM_OFFSET_PX;

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
	// shifts and we may no longer be at the bottom. Re-pin if locked.
	const handleViewportResize = useEffectEvent(() => {
		const s = stateRef.current;
		if (!s.scrollElement || !s.internalIsAtBottom) {
			return;
		}

		scrollTo(s, maxScrollTop(s));
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

		if (s.viewportObserver) {
			s.viewportObserver.disconnect();
			s.viewportObserver = null;
		}

		s.scrollElement = el;

		if (el) {
			el.addEventListener("scroll", handleScroll, { passive: true });
			el.addEventListener("wheel", handleWheel, { passive: true });

			const vo = new ResizeObserver(handleViewportResize);
			vo.observe(el);
			s.viewportObserver = vo;
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
			if (s.viewportObserver) {
				s.viewportObserver.disconnect();
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
	const { scrollRef, contentRef, scrollToBottom, isAtBottom } =
		useStickToBottom();

	// Merge our callback ref with the external RefObject so both
	// point at the same DOM node, and expose scrollToBottom to the
	// parent via its imperative ref.
	const mergedScrollRef = useEffectEvent((el: HTMLDivElement | null) => {
		scrollRef(el);
		scrollContainerRef.current = el;
		scrollToBottomRef.current = el ? () => scrollToBottom("instant") : null;
	});

	// -------------------------------------------------------------------
	// Pagination sentinel (IntersectionObserver)
	// -------------------------------------------------------------------

	const sentinelRef = useRef<HTMLDivElement>(null);
	const observerRef = useRef<IntersectionObserver | null>(null);
	const isFetchingRef = useRef(isFetchingMoreMessages);
	const hasFetchedRef = useRef(false);

	useLayoutEffect(() => {
		isFetchingRef.current = isFetchingMoreMessages;
		if (isFetchingMoreMessages) {
			hasFetchedRef.current = true;
		}
	}, [isFetchingMoreMessages]);

	// Snapshot captured before a fetch so we can restore scroll
	// position after older messages are prepended.
	const pendingPrependRef = useRef<{
		scrollHeight: number;
	} | null>(null);

	useEffect(() => {
		const sentinel = sentinelRef.current;
		const container = scrollContainerRef.current;
		if (!sentinel || !container) return;

		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry.isIntersecting && !isFetchingRef.current) {
					const container = scrollContainerRef.current;
					if (container) {
						pendingPrependRef.current = {
							scrollHeight: container.scrollHeight,
						};
					}
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
	// Prepend scroll restoration
	// -------------------------------------------------------------------

	// When older messages are prepended the browser keeps scrollTop
	// constant while scrollHeight grows, shifting the viewport down.
	// Compensate by adding the height delta to scrollTop.
	useLayoutEffect(() => {
		if (isFetchingMoreMessages) return;
		const pending = pendingPrependRef.current;
		const container = scrollContainerRef.current;
		if (!pending || !container) return;

		const delta = container.scrollHeight - pending.scrollHeight;
		if (delta > 0) {
			container.scrollTop += delta;
		}
		pendingPrependRef.current = null;
	}, [isFetchingMoreMessages, scrollContainerRef]);

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
