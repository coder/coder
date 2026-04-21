import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type RefCallback,
	useEffect,
	useEffectEvent,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import {
	type AnchorSnapshot,
	CHAT_ANCHOR_SELECTOR,
	CHAT_NON_ANCHOR_SELECTOR,
	canElementScrollInDirection,
	FOLLOW_THRESHOLD_PX,
	findNearestScrollableAncestor,
	getBottomGap,
	getScrollMode,
	resolveAnchorTarget,
	restoreAnchorScrollTop,
	type ScrollMode,
} from "./chatViewportUtils";

const ANCHOR_SELECTOR = CHAT_ANCHOR_SELECTOR;
const NON_ANCHOR_SELECTOR = CHAT_NON_ANCHOR_SELECTOR;
const HISTORY_ROOT_MARGIN = "600px 0px 0px 0px";
const DEFERRED_PIN_FRAME_COUNT = 8;

interface ViewportController {
	scrollRef: RefCallback<HTMLDivElement>;
	contentRef: RefCallback<HTMLDivElement>;
	topSentinelRef: RefCallback<HTMLDivElement>;
	bottomSentinelRef: RefCallback<HTMLDivElement>;
	scrollToBottom: (behavior?: ScrollBehavior) => void;
	mode: ScrollMode;
}

const isAnchorCandidate = (element: Element): element is HTMLElement => {
	if (!(element instanceof HTMLElement)) {
		return false;
	}
	if (element.closest(NON_ANCHOR_SELECTOR)) {
		return false;
	}
	return element.dataset.chatAnchor === "true";
};

const findAnchorSnapshot = (
	container: HTMLElement,
	content: HTMLElement,
): AnchorSnapshot | null => {
	const containerTop = container.getBoundingClientRect().top;
	const anchors = content.querySelectorAll(ANCHOR_SELECTOR);
	for (const anchor of anchors) {
		if (!isAnchorCandidate(anchor)) {
			continue;
		}
		const rect = anchor.getBoundingClientRect();
		if (rect.bottom > containerTop + 1) {
			return {
				anchorId: anchor.dataset.chatAnchorId ?? "",
				offsetTop: rect.top - containerTop,
			};
		}
	}
	return null;
};

function useChatViewportController({
	hasMoreMessages,
	isFetchingMoreMessages,
	onFetchMoreMessages,
}: {
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	onFetchMoreMessages: () => void;
}): ViewportController {
	const scrollElementRef = useRef<HTMLDivElement | null>(null);
	const contentElementRef = useRef<HTMLDivElement | null>(null);
	const topSentinelElementRef = useRef<HTMLDivElement | null>(null);
	const bottomSentinelElementRef = useRef<HTMLDivElement | null>(null);
	const [mode, setMode] = useState<ScrollMode>("following-latest");

	const modeRef = useRef<ScrollMode>("following-latest");
	const anchorRef = useRef<AnchorSnapshot | null>(null);
	const previousContentHeightRef = useRef<number | null>(null);
	const previousScrollTopRef = useRef(0);
	const isProgrammaticScrollRef = useRef(false);
	const deferredPinFrameRef = useRef<number | null>(null);
	const isRestoringRef = useRef(false);
	const isPointerDownRef = useRef(false);
	const isTouchActiveRef = useRef(false);
	const activeTouchCountRef = useRef(0);
	const isFetchingHistoryRef = useRef(false);
	const topObserverRef = useRef<IntersectionObserver | null>(null);

	const scrollRef: RefCallback<HTMLDivElement> = (element) => {
		if (scrollElementRef.current === element) {
			return;
		}
		scrollElementRef.current = element;
	};
	const contentRef: RefCallback<HTMLDivElement> = (element) => {
		if (contentElementRef.current === element) {
			return;
		}
		contentElementRef.current = element;
	};
	const topSentinelRef: RefCallback<HTMLDivElement> = (element) => {
		if (topSentinelElementRef.current === element) {
			return;
		}
		topSentinelElementRef.current = element;
	};
	const bottomSentinelRef: RefCallback<HTMLDivElement> = (element) => {
		if (bottomSentinelElementRef.current === element) {
			return;
		}
		bottomSentinelElementRef.current = element;
	};

	const syncMode = useEffectEvent((nextMode: ScrollMode) => {
		modeRef.current = nextMode;
		setMode(nextMode);
	});

	const cancelDeferredPin = useEffectEvent(() => {
		if (deferredPinFrameRef.current !== null) {
			cancelAnimationFrame(deferredPinFrameRef.current);
			deferredPinFrameRef.current = null;
		}
	});

	const pinToLatest = useEffectEvent(() => {
		const container = scrollElementRef.current;
		if (!container) {
			return;
		}
		container.scrollTop = container.scrollHeight;
		previousScrollTopRef.current = container.scrollTop;
	});

	const scheduleDeferredPinToLatest = useEffectEvent((frames: number) => {
		cancelDeferredPin();
		const step = (remainingFrames: number) => {
			const container = scrollElementRef.current;
			if (!container || modeRef.current !== "following-latest") {
				deferredPinFrameRef.current = null;
				return;
			}
			pinToLatest();
			if (remainingFrames <= 0) {
				deferredPinFrameRef.current = null;
				return;
			}
			deferredPinFrameRef.current = requestAnimationFrame(() => {
				step(remainingFrames - 1);
			});
		};
		step(frames);
	});

	const scrollToBottom = useEffectEvent(
		(behavior: ScrollBehavior = "smooth") => {
			const container = scrollElementRef.current;
			if (!container) {
				return;
			}
			anchorRef.current = null;
			syncMode("following-latest");
			if (behavior === "instant") {
				isProgrammaticScrollRef.current = false;
				scheduleDeferredPinToLatest(DEFERRED_PIN_FRAME_COUNT);
				return;
			}
			cancelDeferredPin();
			isProgrammaticScrollRef.current = true;
			container.scrollTo({ top: container.scrollHeight, behavior });
		},
	);

	const restoreAnchor = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}
		const snapshot = anchorRef.current;
		if (!snapshot?.anchorId) {
			return;
		}
		const resolvedTarget = resolveAnchorTarget({
			content: contentElement,
			anchorId: snapshot.anchorId,
		});
		if (!resolvedTarget) {
			return;
		}
		const { anchorId, element: anchor } = resolvedTarget;
		if (anchorId !== snapshot.anchorId) {
			anchorRef.current = { ...snapshot, anchorId };
		}
		const containerTop = scrollElement.getBoundingClientRect().top;
		const nextScrollTop = restoreAnchorScrollTop({
			currentScrollTop: scrollElement.scrollTop,
			currentAnchorTop: anchor.getBoundingClientRect().top - containerTop,
			targetOffsetTop: snapshot.offsetTop,
		});
		if (Math.abs(nextScrollTop - scrollElement.scrollTop) <= 1) {
			return;
		}
		isRestoringRef.current = true;
		scrollElement.scrollTop = nextScrollTop;
		previousScrollTopRef.current = scrollElement.scrollTop;
		requestAnimationFrame(() => {
			isRestoringRef.current = false;
		});
	});

	const syncViewportToCurrentMode = useEffectEvent(() => {
		if (modeRef.current === "following-latest") {
			if (isPointerDownRef.current || isTouchActiveRef.current) {
				return;
			}
			anchorRef.current = null;
			isProgrammaticScrollRef.current = false;
			pinToLatest();
			return;
		}
		restoreAnchor();
	});

	useEffect(() => {
		return () => {
			if (deferredPinFrameRef.current !== null) {
				cancelAnimationFrame(deferredPinFrameRef.current);
				deferredPinFrameRef.current = null;
			}
		};
	}, []);

	useEffect(() => {
		isFetchingHistoryRef.current = isFetchingMoreMessages;
	}, [isFetchingMoreMessages]);

	useEffect(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}

		const updateModeFromGeometry = () => {
			if (isRestoringRef.current) {
				return;
			}
			const bottomGap = getBottomGap({
				scrollHeight: scrollElement.scrollHeight,
				clientHeight: scrollElement.clientHeight,
				scrollTop: scrollElement.scrollTop,
			});
			const previousScrollTop = previousScrollTopRef.current;
			const currentScrollTop = scrollElement.scrollTop;
			previousScrollTopRef.current = currentScrollTop;

			if (isProgrammaticScrollRef.current) {
				if (bottomGap <= 1) {
					isProgrammaticScrollRef.current = false;
				} else {
					if (modeRef.current !== "following-latest") {
						syncMode("following-latest");
					}
					anchorRef.current = null;
					return;
				}
			}

			if (
				isTouchActiveRef.current &&
				modeRef.current === "following-latest" &&
				currentScrollTop < previousScrollTop
			) {
				detachViewport();
				return;
			}

			const movingDown = currentScrollTop > previousScrollTop;
			const nextMode =
				modeRef.current === "following-latest"
					? getScrollMode(bottomGap, FOLLOW_THRESHOLD_PX)
					: bottomGap <= 1 || (movingDown && bottomGap <= FOLLOW_THRESHOLD_PX)
						? "following-latest"
						: "detached";
			if (nextMode !== modeRef.current) {
				syncMode(nextMode);
			}
			if (nextMode === "detached") {
				anchorRef.current = findAnchorSnapshot(scrollElement, contentElement);
			} else {
				anchorRef.current = null;
			}
		};

		const handleScroll = () => {
			requestAnimationFrame(updateModeFromGeometry);
		};

		const detachViewport = () => {
			cancelDeferredPin();
			isProgrammaticScrollRef.current = false;
			if (modeRef.current !== "detached") {
				syncMode("detached");
			}
			anchorRef.current = findAnchorSnapshot(scrollElement, contentElement);
		};

		const handleWheel = (event: WheelEvent) => {
			const nestedScroller = findNearestScrollableAncestor(
				event.target,
				scrollElement,
			);
			if (
				nestedScroller &&
				nestedScroller !== scrollElement &&
				canElementScrollInDirection(nestedScroller, event.deltaY)
			) {
				return;
			}
			if (
				event.deltaY < 0 ||
				isPointerDownRef.current ||
				isTouchActiveRef.current
			) {
				detachViewport();
			}
		};

		const handlePointerDown = (event: PointerEvent) => {
			isPointerDownRef.current = event.target === scrollElement;
			if (isPointerDownRef.current) {
				detachViewport();
			}
		};

		const handlePointerUp = () => {
			isPointerDownRef.current = false;
		};

		const handleTouchStart = (event: TouchEvent) => {
			cancelDeferredPin();
			activeTouchCountRef.current += Math.max(1, event.changedTouches.length);
			isTouchActiveRef.current = true;
		};

		const handleTouchEnd = (event: TouchEvent) => {
			activeTouchCountRef.current = Math.max(
				0,
				activeTouchCountRef.current - Math.max(1, event.changedTouches.length),
			);
			isTouchActiveRef.current = activeTouchCountRef.current > 0;
		};

		const handleVisibilityChange = () => {
			if (!document.hidden) {
				return;
			}
			activeTouchCountRef.current = 0;
			isTouchActiveRef.current = false;
		};

		scrollElement.addEventListener("scroll", handleScroll, { passive: true });
		scrollElement.addEventListener("wheel", handleWheel, { passive: true });
		scrollElement.addEventListener("pointerdown", handlePointerDown);
		window.addEventListener("pointerup", handlePointerUp);
		scrollElement.addEventListener("touchstart", handleTouchStart, {
			passive: true,
		});
		scrollElement.addEventListener("touchend", handleTouchEnd, {
			passive: true,
		});
		scrollElement.addEventListener("touchcancel", handleTouchEnd, {
			passive: true,
		});
		document.addEventListener("visibilitychange", handleVisibilityChange);

		updateModeFromGeometry();

		return () => {
			scrollElement.removeEventListener("scroll", handleScroll);
			scrollElement.removeEventListener("wheel", handleWheel);
			scrollElement.removeEventListener("pointerdown", handlePointerDown);
			window.removeEventListener("pointerup", handlePointerUp);
			scrollElement.removeEventListener("touchstart", handleTouchStart);
			scrollElement.removeEventListener("touchend", handleTouchEnd);
			scrollElement.removeEventListener("touchcancel", handleTouchEnd);
			document.removeEventListener("visibilitychange", handleVisibilityChange);
		};
	}, []);

	useLayoutEffect(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}

		// WebKit can paint one frame of stale scroll geometry after transcript
		// commits or parent layout shifts before ResizeObserver delivers. Apply
		// the current scroll policy during layout so pinned and detached views do
		// not visibly jump between React-driven commits.
		syncViewportToCurrentMode();
	});

	useLayoutEffect(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}

		previousContentHeightRef.current =
			contentElement.getBoundingClientRect().height;
		if (modeRef.current === "following-latest") {
			anchorRef.current = null;
			isProgrammaticScrollRef.current = false;
			scheduleDeferredPinToLatest(DEFERRED_PIN_FRAME_COUNT);
		}

		const mutationObserver = new MutationObserver(() => {
			// Reading the content box here forces WebKit to resolve layout before
			// we decide whether to pin or restore, which avoids a one-frame gap
			// after transcript mutations.
			previousContentHeightRef.current =
				contentElement.getBoundingClientRect().height;
			syncViewportToCurrentMode();
		});
		mutationObserver.observe(contentElement, {
			childList: true,
			subtree: true,
			characterData: true,
		});

		const resizeObserver = new ResizeObserver(() => {
			const nextHeight = contentElement.getBoundingClientRect().height;
			const previousHeight = previousContentHeightRef.current;
			previousContentHeightRef.current = nextHeight;
			if (previousHeight === null) {
				return;
			}
			if (modeRef.current === "following-latest") {
				if (isPointerDownRef.current || isTouchActiveRef.current) {
					return;
				}
				anchorRef.current = null;
				isProgrammaticScrollRef.current = false;
				scheduleDeferredPinToLatest(DEFERRED_PIN_FRAME_COUNT);
				return;
			}
			restoreAnchor();
		});
		resizeObserver.observe(contentElement);
		resizeObserver.observe(scrollElement);
		return () => {
			mutationObserver.disconnect();
			resizeObserver.disconnect();
		};
	}, []);

	useEffect(() => {
		const scrollElement = scrollElementRef.current;
		const topSentinelElement = topSentinelElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !topSentinelElement || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			([entry]) => {
				const canFetchWithoutDetaching =
					scrollElement.scrollHeight <= scrollElement.clientHeight + 1;
				if (
					!entry?.isIntersecting ||
					isFetchingHistoryRef.current ||
					(modeRef.current !== "detached" && !canFetchWithoutDetaching)
				) {
					return;
				}
				if (contentElement && modeRef.current === "detached") {
					anchorRef.current = findAnchorSnapshot(scrollElement, contentElement);
				}
				onFetchMoreMessages();
			},
			{
				root: scrollElement,
				rootMargin: HISTORY_ROOT_MARGIN,
				threshold: 0.01,
			},
		);
		observer.observe(topSentinelElement);
		topObserverRef.current = observer;
		return () => {
			observer.disconnect();
			topObserverRef.current = null;
		};
	}, [hasMoreMessages, onFetchMoreMessages]);

	useEffect(() => {
		const observer = topObserverRef.current;
		const topSentinelElement = topSentinelElementRef.current;
		if (isFetchingMoreMessages || !observer || !topSentinelElement) {
			return;
		}
		observer.unobserve(topSentinelElement);
		observer.observe(topSentinelElement);
	}, [isFetchingMoreMessages]);

	useEffect(() => {
		const observer = topObserverRef.current;
		const topSentinelElement = topSentinelElementRef.current;
		if (
			mode !== "detached" ||
			!observer ||
			!topSentinelElement ||
			isFetchingMoreMessages
		) {
			return;
		}
		observer.unobserve(topSentinelElement);
		observer.observe(topSentinelElement);
	}, [isFetchingMoreMessages, mode]);

	useLayoutEffect(() => {
		const scrollElement = scrollElementRef.current;
		const bottomSentinelElement = bottomSentinelElementRef.current;
		if (!scrollElement || !bottomSentinelElement) {
			return;
		}
		if (modeRef.current !== "following-latest") {
			return;
		}
		scheduleDeferredPinToLatest(DEFERRED_PIN_FRAME_COUNT);
	}, []);

	return {
		scrollRef,
		contentRef,
		topSentinelRef,
		bottomSentinelRef,
		scrollToBottom,
		mode,
	};
}

const ChatScrollContainer: FC<{
	onScrollContainerChange?: (element: HTMLDivElement | null) => void;
	onScrollToBottomChange?: (scrollToBottom: (() => void) | null) => void;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: React.ReactNode;
}> = ({
	onScrollContainerChange,
	onScrollToBottomChange,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const {
		scrollRef,
		contentRef,
		topSentinelRef,
		bottomSentinelRef,
		scrollToBottom,
		mode,
	} = useChatViewportController({
		hasMoreMessages,
		isFetchingMoreMessages,
		onFetchMoreMessages,
	});

	const handleScrollContainerRef: RefCallback<HTMLDivElement> = (element) => {
		scrollRef(element);
		onScrollContainerChange?.(element);
		onScrollToBottomChange?.(element ? () => scrollToBottom("instant") : null);
	};

	const showButton = mode === "detached";

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={handleScrollContainerRef}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [overscroll-behavior:contain] [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div ref={contentRef}>
					{hasMoreMessages ? (
						<div ref={topSentinelRef} className="h-px shrink-0" />
					) : null}
					{children}
					<div ref={bottomSentinelRef} className="h-px shrink-0" />
				</div>
			</div>
			<div
				data-chat-anchor-ignore="true"
				className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center overflow-y-auto py-2 [scrollbar-gutter:stable] [scrollbar-width:thin]"
			>
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
