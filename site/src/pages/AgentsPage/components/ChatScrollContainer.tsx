import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import InfiniteScroll from "react-infinite-scroll-component";
import { Button } from "#/components/Button/Button";
import type { ChatViewportAnchor } from "../chatSession/types";

const SCROLL_THRESHOLD = "600px";
const CHAT_BOTTOM_THRESHOLD_PX = 70;
const MESSAGE_ANCHOR_SELECTOR = "[data-chat-message-anchor]";

type ChatScrollContainerProps = {
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	messageCount: number;
	children: ReactNode;
	followMode: boolean;
	hasNewOffscreenContent: boolean;
	viewportAnchor: ChatViewportAnchor | null;
	onFollowModeChange: (next: boolean) => void;
	onViewportAnchorChange: (anchor: ChatViewportAnchor | null) => void;
	onClearNewOffscreenContent: () => void;
	getNewestMessageId: () => number | undefined;
};

type LatestProps = Pick<
	ChatScrollContainerProps,
	| "followMode"
	| "viewportAnchor"
	| "getNewestMessageId"
	| "onFollowModeChange"
	| "onViewportAnchorChange"
	| "onClearNewOffscreenContent"
>;

const areViewportAnchorsEqual = (
	left: ChatViewportAnchor | null,
	right: ChatViewportAnchor | null,
): boolean => {
	if (left === right) {
		return true;
	}
	if (!left || !right) {
		return false;
	}
	return (
		left.messageId === right.messageId &&
		left.offsetTop === right.offsetTop &&
		left.newestMessageIdAtCapture === right.newestMessageIdAtCapture
	);
};

const captureTopVisibleAnchor = (
	scroller: HTMLDivElement,
	getNewestMessageId: () => number | undefined,
): ChatViewportAnchor | null => {
	const scrollerRect = scroller.getBoundingClientRect();
	const anchorElements = scroller.querySelectorAll<HTMLElement>(
		MESSAGE_ANCHOR_SELECTOR,
	);

	for (const anchorElement of anchorElements) {
		const anchorRect = anchorElement.getBoundingClientRect();
		const topInsideViewport =
			anchorRect.top >= scrollerRect.top &&
			anchorRect.top <= scrollerRect.bottom;
		const crossesViewportTop =
			anchorRect.top <= scrollerRect.top &&
			anchorRect.bottom >= scrollerRect.top;

		if (!topInsideViewport && !crossesViewportTop) {
			continue;
		}

		const messageId = Number(anchorElement.dataset.chatMessageId);
		if (!Number.isFinite(messageId)) {
			continue;
		}

		return {
			messageId,
			offsetTop: anchorRect.top - scrollerRect.top,
			newestMessageIdAtCapture: getNewestMessageId(),
		};
	}

	return null;
};

const restoreViewportAnchor = (
	scroller: HTMLDivElement,
	anchor: ChatViewportAnchor,
): void => {
	const anchorElement = scroller.querySelector<HTMLElement>(
		`[data-chat-message-anchor][data-chat-message-id="${anchor.messageId}"]`,
	);
	if (!anchorElement) {
		return;
	}

	const scrollerRect = scroller.getBoundingClientRect();
	const anchorRect = anchorElement.getBoundingClientRect();
	const delta = anchorRect.top - scrollerRect.top - anchor.offsetTop;
	if (Math.abs(delta) <= 0.5) {
		return;
	}

	scroller.scrollTop += delta;
};

const pinToBottom = (scroller: HTMLDivElement): void => {
	scroller.scrollTop = 0;
};

const pinToBottomIfNeeded = (scroller: HTMLDivElement): void => {
	if (Math.abs(scroller.scrollTop) <= 0.5) {
		return;
	}
	pinToBottom(scroller);
};

const ScrollToBottomButton: FC<{
	followMode: boolean;
	hasNewOffscreenContent: boolean;
	onScrollToBottom: () => void;
}> = ({ followMode, hasNewOffscreenContent, onScrollToBottom }) => {
	if (followMode) {
		return null;
	}

	const label = hasNewOffscreenContent ? "New messages" : "Scroll to bottom";

	return (
		<div className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center overflow-y-auto py-2 [scrollbar-gutter:stable] [scrollbar-width:thin]">
			<Button
				variant="outline"
				size={hasNewOffscreenContent ? "sm" : "icon"}
				className="pointer-events-auto rounded-full bg-surface-primary shadow-md transition-all duration-200"
				onClick={onScrollToBottom}
				aria-label={label}
			>
				{hasNewOffscreenContent && <span>{label}</span>}
				<ArrowDownIcon />
			</Button>
		</div>
	);
};

const ChatScrollContainer: FC<ChatScrollContainerProps> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	messageCount,
	children,
	followMode,
	hasNewOffscreenContent,
	viewportAnchor,
	onFollowModeChange,
	onViewportAnchorChange,
	onClearNewOffscreenContent,
	getNewestMessageId,
}) => {
	const [scrollContainerElement, setScrollContainerElement] =
		useState<HTMLDivElement | null>(null);
	const [contentElement, setContentElement] = useState<HTMLDivElement | null>(
		null,
	);
	const latestPropsRef = useRef<LatestProps>({
		followMode,
		viewportAnchor,
		getNewestMessageId,
		onFollowModeChange,
		onViewportAnchorChange,
		onClearNewOffscreenContent,
	});

	useLayoutEffect(() => {
		latestPropsRef.current = {
			followMode,
			viewportAnchor,
			getNewestMessageId,
			onFollowModeChange,
			onViewportAnchorChange,
			onClearNewOffscreenContent,
		};
	});

	const scrollToBottom = () => {
		// Read the live ref so remounts cannot leave callers targeting a detached
		// scroll node.
		const scrollContainer = scrollContainerRef.current;
		if (!scrollContainer) {
			return;
		}
		// In the library's reversed layout, the newest messages sit at the visual
		// bottom, which maps to a zero scroll offset.
		pinToBottom(scrollContainer);
	};

	const setScrollContainer = (element: HTMLDivElement | null) => {
		scrollContainerRef.current = element;
		if (element && latestPropsRef.current.followMode) {
			pinToBottom(element);
		}
		setScrollContainerElement(element);
		scrollToBottomRef.current = element ? scrollToBottom : null;
	};

	useLayoutEffect(() => {
		if (!scrollContainerElement) {
			return;
		}

		if (followMode) {
			pinToBottom(scrollContainerElement);
			return;
		}

		if (messageCount === 0) {
			return;
		}

		if (viewportAnchor) {
			restoreViewportAnchor(scrollContainerElement, viewportAnchor);
		}
	}, [scrollContainerElement, followMode, viewportAnchor, messageCount]);

	useEffect(() => {
		if (!scrollContainerElement || !contentElement) {
			return;
		}
		if (typeof ResizeObserver === "undefined") {
			return;
		}

		let frameId: number | null = null;
		const observer = new ResizeObserver(() => {
			if (frameId !== null) {
				return;
			}
			frameId = requestAnimationFrame(() => {
				frameId = null;
				const latest = latestPropsRef.current;
				if (latest.followMode) {
					pinToBottomIfNeeded(scrollContainerElement);
					return;
				}
				if (latest.viewportAnchor) {
					restoreViewportAnchor(scrollContainerElement, latest.viewportAnchor);
				}
			});
		});

		observer.observe(contentElement);
		return () => {
			observer.disconnect();
			if (frameId !== null) {
				cancelAnimationFrame(frameId);
			}
		};
	}, [scrollContainerElement, contentElement]);

	useEffect(() => {
		if (!scrollContainerElement) {
			return;
		}

		let frameId: number | null = null;
		const handleScroll = () => {
			if (frameId !== null) {
				return;
			}
			frameId = requestAnimationFrame(() => {
				frameId = null;
				const latest = latestPropsRef.current;
				const atBottom =
					Math.abs(scrollContainerElement.scrollTop) <=
					CHAT_BOTTOM_THRESHOLD_PX;

				if (atBottom !== latest.followMode) {
					latest.onFollowModeChange(atBottom);
					latestPropsRef.current = {
						...latestPropsRef.current,
						followMode: atBottom,
					};
				}

				if (atBottom) {
					latestPropsRef.current.onClearNewOffscreenContent();
					return;
				}

				const nextAnchor = captureTopVisibleAnchor(
					scrollContainerElement,
					latestPropsRef.current.getNewestMessageId,
				);
				if (
					areViewportAnchorsEqual(
						nextAnchor,
						latestPropsRef.current.viewportAnchor,
					)
				) {
					return;
				}

				latestPropsRef.current.onViewportAnchorChange(nextAnchor);
				latestPropsRef.current = {
					...latestPropsRef.current,
					viewportAnchor: nextAnchor,
				};
			});
		};

		scrollContainerElement.addEventListener("scroll", handleScroll, {
			passive: true,
		});
		return () => {
			scrollContainerElement.removeEventListener("scroll", handleScroll);
			if (frameId !== null) {
				cancelAnimationFrame(frameId);
			}
		};
	}, [scrollContainerElement]);

	const handleScrollToBottom = () => {
		scrollToBottom();
		onFollowModeChange(true);
		onClearNewOffscreenContent();
	};

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={setScrollContainer}
				data-testid="scroll-container"
				aria-busy={isFetchingMoreMessages || undefined}
				className="flex min-h-0 flex-1 flex-col-reverse overflow-y-auto [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
			>
				<div aria-hidden className="flex-1 basis-0" />
				<InfiniteScroll
					dataLength={messageCount}
					next={onFetchMoreMessages}
					hasMore={hasMoreMessages}
					inverse
					scrollableTarget={scrollContainerElement ?? undefined}
					scrollThreshold={SCROLL_THRESHOLD}
					hasChildren={messageCount > 0}
					loader={isFetchingMoreMessages ? <div aria-hidden /> : null}
					endMessage={null}
					style={{ display: "flex", flexDirection: "column-reverse" }}
				>
					<div ref={setContentElement}>{children}</div>
				</InfiniteScroll>
			</div>
			<ScrollToBottomButton
				followMode={followMode}
				hasNewOffscreenContent={hasNewOffscreenContent}
				onScrollToBottom={handleScrollToBottom}
			/>
		</div>
	);
};

export { ChatScrollContainer };
