import {
	type FC,
	type ReactNode,
	type RefObject,
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
} from "react";
import { ScrollToBottomButton } from "./ScrollToBottomButton";
import { useScrollAnchor } from "./useScrollAnchor";

const LOAD_MORE_MARGIN = "600px 0px 0px 0px";

type ChatVirtualListProps = {
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	messageCount: number;
	children: ReactNode;
};

export const ChatVirtualList: FC<ChatVirtualListProps> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	messageCount,
	children,
}) => {
	const { scrollerRef, contentRef, atBottom, scrollToBottom, maintainPin } =
		useScrollAnchor();
	const topSentinelRef = useRef<HTMLDivElement | null>(null);

	const setScroller = useCallback(
		(element: HTMLDivElement | null) => {
			scrollerRef.current = element;
			scrollContainerRef.current = element;
			scrollToBottomRef.current = element ? scrollToBottom : null;
		},
		[scrollerRef, scrollContainerRef, scrollToBottomRef, scrollToBottom],
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies(messageCount): messageCount is an intentional trigger so a new message pins to the bottom; maintainPin reads refs and must re-run when the count changes.
	useLayoutEffect(() => {
		maintainPin();
	}, [messageCount, maintainPin]);

	useEffect(() => {
		const sentinel = topSentinelRef.current;
		const scroller = scrollerRef.current;
		if (!sentinel || !scroller || !hasMoreMessages) {
			return;
		}
		const observer = new IntersectionObserver(
			([entry]) => {
				if (
					entry.isIntersecting &&
					hasMoreMessages &&
					!isFetchingMoreMessages
				) {
					onFetchMoreMessages();
				}
			},
			{ root: scroller, rootMargin: LOAD_MORE_MARGIN },
		);
		observer.observe(sentinel);
		return () => observer.disconnect();
	}, [
		scrollerRef,
		hasMoreMessages,
		isFetchingMoreMessages,
		onFetchMoreMessages,
	]);

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={setScroller}
				data-testid="scroll-container"
				className="flex min-h-0 flex-1 flex-col overflow-y-auto [overflow-anchor:none] [scrollbar-gutter:stable]"
			>
				<div ref={contentRef} className="flex flex-col">
					<div ref={topSentinelRef} aria-hidden className="h-0" />
					{children}
				</div>
			</div>
			<ScrollToBottomButton
				visible={!atBottom}
				onScrollToBottom={scrollToBottom}
			/>
		</div>
	);
};
