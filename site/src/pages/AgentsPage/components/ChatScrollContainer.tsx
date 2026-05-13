import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
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
	FOLLOW_THRESHOLD_PX,
	getBottomGap,
	getScrollMode,
	resolveAnchorTarget,
	restoreAnchorScrollTop,
	type ScrollDirection,
	type ScrollMode,
} from "./chatViewportUtils";

const HISTORY_ROOT_MARGIN = "600px 0px 0px 0px";
const HISTORY_FETCH_SCROLL_THRESHOLD_PX = 600;
const DEFERRED_PIN_FRAME_COUNT = 8;

const ScrollToBottomButton: FC<{
	show: boolean;
	onScrollToBottom: () => void;
}> = ({ show, onScrollToBottom }) => {
	return (
		<div
			data-chat-anchor-ignore="true"
			className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center py-2"
		>
			<Button
				variant="outline"
				size="icon"
				className={cn(
					"rounded-full bg-surface-primary shadow-md transition-all duration-200",
					show
						? "pointer-events-auto translate-y-0 opacity-100"
						: "translate-y-2 opacity-0",
				)}
				onClick={onScrollToBottom}
				aria-label="Scroll to bottom"
				aria-hidden={!show || undefined}
				tabIndex={show ? undefined : -1}
			>
				<ArrowDownIcon />
			</Button>
		</div>
	);
};

const isAnchorCandidate = (element: Element): element is HTMLElement => {
	if (!(element instanceof HTMLElement)) {
		return false;
	}
	if (element.closest(CHAT_NON_ANCHOR_SELECTOR)) {
		return false;
	}
	return element.dataset.chatAnchor === "true";
};

const findAnchorSnapshot = (
	container: HTMLElement,
	content: HTMLElement,
): AnchorSnapshot | null => {
	const containerRect = container.getBoundingClientRect();
	const anchors = content.querySelectorAll(CHAT_ANCHOR_SELECTOR);
	for (const anchor of anchors) {
		if (!isAnchorCandidate(anchor)) {
			continue;
		}
		const rect = anchor.getBoundingClientRect();
		if (
			rect.bottom > containerRect.top + 1 &&
			rect.top < containerRect.bottom - 1
		) {
			const anchorId = anchor.dataset.chatAnchorId;
			if (!anchorId) {
				continue;
			}
			return {
				anchorId,
				offsetTop: rect.top - containerRect.top,
			};
		}
	}
	return null;
};

const getDirectionFromWheel = (deltaY: number): ScrollDirection =>
	deltaY < 0 ? "up" : "down";

const ChatScrollContainer: FC<{
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	messageCount: number;
	children: ReactNode;
}> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	messageCount,
	children,
}) => {
	const scrollElementRef = useRef<HTMLDivElement | null>(null);
	const contentElementRef = useRef<HTMLDivElement | null>(null);
	const topSentinelElementRef = useRef<HTMLDivElement | null>(null);
	const [mode, setMode] = useState<ScrollMode>("following-latest");
	const modeRef = useRef<ScrollMode>("following-latest");
	const anchorRef = useRef<AnchorSnapshot | null>(null);
	const previousMessageCountRef = useRef(messageCount);
	const isFetchingHistoryRef = useRef(isFetchingMoreMessages);
	const isProgrammaticScrollRef = useRef(false);
	const isPointerDownRef = useRef(false);
	const activeTouchCountRef = useRef(0);
	const scrollFrameRef = useRef<number | null>(null);
	const restoreFrameRef = useRef<number | null>(null);
	const deferredPinFrameRef = useRef<number | null>(null);

	const syncMode = useEffectEvent((nextMode: ScrollMode) => {
		modeRef.current = nextMode;
		setMode(nextMode);
	});

	const clearDeferredPin = useEffectEvent(() => {
		if (deferredPinFrameRef.current !== null) {
			cancelAnimationFrame(deferredPinFrameRef.current);
			deferredPinFrameRef.current = null;
		}
	});

	const pinToLatest = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		if (!scrollElement) {
			return;
		}
		scrollElement.scrollTop = scrollElement.scrollHeight;
	});

	const scheduleDeferredPin = useEffectEvent(
		(frames = DEFERRED_PIN_FRAME_COUNT) => {
			clearDeferredPin();
			const step = (remainingFrames: number) => {
				const scrollElement = scrollElementRef.current;
				if (!scrollElement || modeRef.current !== "following-latest") {
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
		},
	);

	const captureAnchor = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}
		anchorRef.current = findAnchorSnapshot(scrollElement, contentElement);
	});

	const restoreAnchor = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		const snapshot = anchorRef.current;
		if (!scrollElement || !contentElement || !snapshot) {
			return;
		}
		const target = resolveAnchorTarget(contentElement, snapshot);
		if (!target) {
			return;
		}
		const nextScrollTop = restoreAnchorScrollTop(
			scrollElement,
			target,
			snapshot,
		);
		if (Math.abs(nextScrollTop - scrollElement.scrollTop) <= 1) {
			return;
		}
		isProgrammaticScrollRef.current = true;
		scrollElement.scrollTop = nextScrollTop;
	});

	const requestHistoryFetch = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		if (
			!scrollElement ||
			!hasMoreMessages ||
			isFetchingHistoryRef.current ||
			scrollElement.scrollTop > HISTORY_FETCH_SCROLL_THRESHOLD_PX
		) {
			return;
		}
		const canFetchWhileFollowing =
			scrollElement.scrollHeight <= scrollElement.clientHeight + 1;
		const geometryMode = getScrollMode(getBottomGap(scrollElement));
		const isDetached =
			modeRef.current === "detached" || geometryMode === "detached";
		if (!isDetached && !canFetchWhileFollowing) {
			return;
		}
		if (isDetached) {
			if (modeRef.current !== "detached") {
				syncMode("detached");
			}
			captureAnchor();
		}
		isFetchingHistoryRef.current = true;
		onFetchMoreMessages();
	});

	const syncViewportToMode = useEffectEvent(() => {
		if (modeRef.current === "following-latest") {
			if (isPointerDownRef.current || activeTouchCountRef.current > 0) {
				return;
			}
			anchorRef.current = null;
			scheduleDeferredPin();
			return;
		}
		if (!anchorRef.current) {
			captureAnchor();
		}
		restoreAnchor();
	});

	const scheduleViewportSync = useEffectEvent(() => {
		if (restoreFrameRef.current !== null) {
			return;
		}
		restoreFrameRef.current = requestAnimationFrame(() => {
			restoreFrameRef.current = null;
			syncViewportToMode();
		});
	});

	const updateModeFromGeometry = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}
		const bottomGap = getBottomGap(scrollElement);
		if (isProgrammaticScrollRef.current) {
			isProgrammaticScrollRef.current = false;
			if (bottomGap <= FOLLOW_THRESHOLD_PX) {
				if (modeRef.current !== "following-latest") {
					syncMode("following-latest");
				}
				anchorRef.current = null;
				return;
			}
		}

		const nextMode = getScrollMode(bottomGap);
		if (nextMode !== modeRef.current) {
			syncMode(nextMode);
		}
		if (nextMode === "detached") {
			anchorRef.current = findAnchorSnapshot(scrollElement, contentElement);
		} else {
			anchorRef.current = null;
		}
		requestHistoryFetch();
	});

	const scheduleScrollModeUpdate = useEffectEvent(() => {
		if (scrollFrameRef.current !== null) {
			return;
		}
		scrollFrameRef.current = requestAnimationFrame(() => {
			scrollFrameRef.current = null;
			updateModeFromGeometry();
		});
	});

	const scrollToBottom = useEffectEvent(() => {
		const scrollElement = scrollElementRef.current;
		if (!scrollElement) {
			return;
		}
		anchorRef.current = null;
		isProgrammaticScrollRef.current = true;
		syncMode("following-latest");
		pinToLatest();
	});

	const setScrollContainer = (element: HTMLDivElement | null) => {
		scrollElementRef.current = element;
		scrollContainerRef.current = element;
		scrollToBottomRef.current = element ? scrollToBottom : null;
	};

	const setContentElement = (element: HTMLDivElement | null) => {
		contentElementRef.current = element;
	};

	const setTopSentinel = (element: HTMLDivElement | null) => {
		topSentinelElementRef.current = element;
	};

	useEffect(() => {
		isFetchingHistoryRef.current = isFetchingMoreMessages;
	}, [isFetchingMoreMessages]);

	useEffect(() => {
		return () => {
			if (scrollFrameRef.current !== null) {
				cancelAnimationFrame(scrollFrameRef.current);
			}
			if (restoreFrameRef.current !== null) {
				cancelAnimationFrame(restoreFrameRef.current);
			}
			if (deferredPinFrameRef.current !== null) {
				cancelAnimationFrame(deferredPinFrameRef.current);
			}
			scrollToBottomRef.current = null;
		};
	}, [scrollToBottomRef]);

	useEffect(() => {
		const scrollElement = scrollElementRef.current;
		if (!scrollElement) {
			return;
		}

		const handleScroll = () => {
			if (getBottomGap(scrollElement) > FOLLOW_THRESHOLD_PX) {
				clearDeferredPin();
				if (modeRef.current === "following-latest") {
					syncMode("detached");
					captureAnchor();
				}
			}
			scheduleScrollModeUpdate();
		};
		const handleWheel = (event: WheelEvent) => {
			const direction = getDirectionFromWheel(event.deltaY);
			if (direction === "up") {
				clearDeferredPin();
			}
		};
		const handlePointerDown = (event: PointerEvent) => {
			isPointerDownRef.current = event.target === scrollElement;
			if (isPointerDownRef.current) {
				clearDeferredPin();
			}
		};
		const handlePointerUp = () => {
			isPointerDownRef.current = false;
		};
		const handleTouchStart = (event: TouchEvent) => {
			activeTouchCountRef.current += Math.max(1, event.changedTouches.length);
			clearDeferredPin();
		};
		const handleTouchEnd = (event: TouchEvent) => {
			activeTouchCountRef.current = Math.max(
				0,
				activeTouchCountRef.current - Math.max(1, event.changedTouches.length),
			);
		};
		const handleVisibilityChange = () => {
			if (document.hidden) {
				activeTouchCountRef.current = 0;
				isPointerDownRef.current = false;
			}
		};

		scrollElement.addEventListener("scroll", handleScroll, { passive: true });
		scrollElement.addEventListener("wheel", handleWheel, { passive: true });
		scrollElement.addEventListener("pointerdown", handlePointerDown);
		scrollElement.addEventListener("touchstart", handleTouchStart, {
			passive: true,
		});
		scrollElement.addEventListener("touchend", handleTouchEnd, {
			passive: true,
		});
		scrollElement.addEventListener("touchcancel", handleTouchEnd, {
			passive: true,
		});
		window.addEventListener("pointerup", handlePointerUp);
		document.addEventListener("visibilitychange", handleVisibilityChange);
		updateModeFromGeometry();

		return () => {
			scrollElement.removeEventListener("scroll", handleScroll);
			scrollElement.removeEventListener("wheel", handleWheel);
			scrollElement.removeEventListener("pointerdown", handlePointerDown);
			scrollElement.removeEventListener("touchstart", handleTouchStart);
			scrollElement.removeEventListener("touchend", handleTouchEnd);
			scrollElement.removeEventListener("touchcancel", handleTouchEnd);
			window.removeEventListener("pointerup", handlePointerUp);
			document.removeEventListener("visibilitychange", handleVisibilityChange);
		};
	}, []);

	useLayoutEffect(() => {
		syncViewportToMode();
	});

	useEffect(() => {
		const scrollElement = scrollElementRef.current;
		const contentElement = contentElementRef.current;
		if (!scrollElement || !contentElement) {
			return;
		}

		const mutationObserver = new MutationObserver(scheduleViewportSync);
		mutationObserver.observe(contentElement, {
			childList: true,
			subtree: true,
			characterData: true,
		});

		const resizeObserver = new ResizeObserver(scheduleViewportSync);
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
		if (!scrollElement || !topSentinelElement || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			([entry]) => {
				if (!entry?.isIntersecting) {
					return;
				}
				requestHistoryFetch();
			},
			{
				root: scrollElement,
				rootMargin: HISTORY_ROOT_MARGIN,
				threshold: 0.01,
			},
		);
		observer.observe(topSentinelElement);
		return () => observer.disconnect();
	}, [hasMoreMessages]);

	useEffect(() => {
		if (previousMessageCountRef.current === messageCount) {
			return;
		}
		previousMessageCountRef.current = messageCount;
		isFetchingHistoryRef.current = isFetchingMoreMessages;
		if (modeRef.current === "following-latest") {
			scheduleDeferredPin();
			return;
		}
		scheduleViewportSync();
	});

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={setScrollContainer}
				data-testid="scroll-container"
				aria-busy={isFetchingMoreMessages || undefined}
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [overscroll-behavior:contain] [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div ref={setContentElement} className="flex min-h-full flex-col">
					{hasMoreMessages ? (
						<div ref={setTopSentinel} className="h-px shrink-0" />
					) : null}
					{children}
					<div className="h-px shrink-0" />
				</div>
			</div>
			<ScrollToBottomButton
				show={messageCount > 0 && mode === "detached"}
				onScrollToBottom={scrollToBottom}
			/>
		</div>
	);
};

export { ChatScrollContainer };
