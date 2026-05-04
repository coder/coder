import { ArrowDownIcon } from "lucide-react";
import {
	type FC,
	type ReactNode,
	type RefObject,
	useEffect,
	useState,
} from "react";
import InfiniteScroll from "react-infinite-scroll-component";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";

const SCROLL_THRESHOLD = "600px";
const SCROLL_TO_BOTTOM_BUTTON_OFFSET_PX = 70;

const ScrollToBottomButton: FC<{
	scrollContainerElement: HTMLDivElement | null;
	messageCount: number;
	onScrollToBottom: () => void;
}> = ({ scrollContainerElement, messageCount, onScrollToBottom }) => {
	const [showScrollToBottomButton, setShowScrollToBottomButton] =
		useState(false);

	useEffect(() => {
		if (!scrollContainerElement) {
			setShowScrollToBottomButton(false);
			return;
		}

		let frameId: number | null = null;
		const updateVisibility = () => {
			setShowScrollToBottomButton(
				Math.abs(scrollContainerElement.scrollTop) >
					SCROLL_TO_BOTTOM_BUTTON_OFFSET_PX,
			);
		};
		const handleScroll = () => {
			if (frameId !== null) {
				return;
			}
			frameId = requestAnimationFrame(() => {
				frameId = null;
				updateVisibility();
			});
		};

		updateVisibility();
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

	useEffect(() => {
		if (!scrollContainerElement) {
			return;
		}
		setShowScrollToBottomButton(
			messageCount > 0 &&
				Math.abs(scrollContainerElement.scrollTop) >
					SCROLL_TO_BOTTOM_BUTTON_OFFSET_PX,
		);
	}, [messageCount, scrollContainerElement]);

	const handleScrollToBottom = () => {
		onScrollToBottom();
		setShowScrollToBottomButton(false);
	};

	return (
		// Floating overlay above the scroll container. The button has its own
		// fixed-size box so the wrapper does not need overflow handling.
		<div className="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center py-2">
			<Button
				variant="outline"
				size="icon"
				className={cn(
					"rounded-full bg-surface-primary shadow-md transition-all duration-200",
					showScrollToBottomButton
						? "pointer-events-auto translate-y-0 opacity-100"
						: "translate-y-2 opacity-0",
				)}
				onClick={handleScrollToBottom}
				aria-label="Scroll to bottom"
				aria-hidden={!showScrollToBottomButton || undefined}
				tabIndex={showScrollToBottomButton ? undefined : -1}
			>
				<ArrowDownIcon />
			</Button>
		</div>
	);
};

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
	const [scrollContainerElement, setScrollContainerElement] =
		useState<HTMLDivElement | null>(null);

	const scrollToBottom = () => {
		// Read the live ref so remounts cannot leave callers targeting a detached
		// scroll node.
		const scrollContainer = scrollContainerRef.current;
		if (!scrollContainer) {
			return;
		}
		// In the library's reversed layout, the newest messages sit at the visual
		// bottom, which maps to a zero scroll offset.
		scrollContainer.scrollTop = 0;
	};

	const setScrollContainer = (element: HTMLDivElement | null) => {
		scrollContainerRef.current = element;
		setScrollContainerElement(element);
		scrollToBottomRef.current = element ? scrollToBottom : null;
	};

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={setScrollContainer}
				data-testid="scroll-container"
				aria-busy={isFetchingMoreMessages || undefined}
				// `react-infinite-scroll-component` renders two wrapper divs
				// between this scroller and the rendered messages. Force both
				// out of the layout tree with `display: contents` so that
				// (a) `position: sticky` on a user message resolves against
				// this scroller rather than the inner wrapper (which has
				// `overflow: auto` baked in by the library), and (b) the
				// column-reverse inverse layout places the library's
				// load-more sentinel at the visual top of the content stack.
				className="flex min-h-0 flex-1 flex-col-reverse overflow-y-auto [&>[class$=outerdiv]]:contents [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]"
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
					// `display: contents` removes this wrapper's box from the
					// layout tree. Combined with the `outerdiv:contents`
					// selector on the scroller above, the children render as
					// direct flex items of the scroller so sticky messages
					// can pin to its top edge.
					style={{ display: "contents" }}
				>
					{children}
				</InfiniteScroll>
			</div>
			<ScrollToBottomButton
				scrollContainerElement={scrollContainerElement}
				messageCount={messageCount}
				onScrollToBottom={scrollToBottom}
			/>
		</div>
	);
};

export { ChatScrollContainer };
