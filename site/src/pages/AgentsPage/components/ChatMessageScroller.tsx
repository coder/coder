import {
	MessageScroller,
	useMessageScrollerScrollable,
} from "@shadcn/react/message-scroller";
import { ArrowDownIcon } from "lucide-react";
import { type FC, type ReactNode, useEffect } from "react";
import { cn } from "#/utils/cn";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";

// preserveScrollOnPrepend keeps the reader's place when older pages mount, and
// the library reports the start edge, so reaching the top is the only trigger
// needed here. No scroll-position math of its own.
const LoadEarlierOnReachStart: FC<{
	hasMore: boolean;
	isFetching: boolean;
	onLoadMore: () => void;
}> = ({ hasMore, isFetching, onLoadMore }) => {
	const { start } = useMessageScrollerScrollable();
	const atStart = !start;

	useEffect(() => {
		if (atStart && hasMore && !isFetching) {
			onLoadMore();
		}
	}, [atStart, hasMore, isFetching, onLoadMore]);

	return null;
};

interface ChatMessageScrollerProps {
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: ReactNode;
}

export const ChatMessageScroller: FC<ChatMessageScrollerProps> = ({
	hasMoreMessages,
	isFetchingMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const [chatFullWidth] = useChatFullWidth();

	return (
		<MessageScroller.Provider
			autoScroll
			defaultScrollPosition="last-anchor"
			scrollPreviousItemPeek={64}
		>
			<MessageScroller.Root className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
				<MessageScroller.Viewport className="size-full min-h-0 overflow-y-auto overscroll-contain [overflow-anchor:none] [scrollbar-color:hsl(var(--surface-quaternary))_transparent] [scrollbar-gutter:stable] [scrollbar-width:thin]">
					<MessageScroller.Content
						data-testid="conversation-timeline"
						aria-busy={isFetchingMoreMessages || undefined}
						className={cn(
							"mx-auto flex h-max min-h-full w-full flex-col gap-2 px-4 py-6",
							chatWidthClass(chatFullWidth),
						)}
					>
						{children}
					</MessageScroller.Content>
				</MessageScroller.Viewport>
				<MessageScroller.Button
					direction="end"
					aria-label="Scroll to bottom"
					className={cn(
						"absolute bottom-4 left-1/2 z-10 -translate-x-1/2",
						"flex size-9 items-center justify-center rounded-full",
						"border border-border-default border-solid bg-surface-primary text-content-primary shadow-md",
						"transition-opacity duration-200 hover:bg-surface-secondary",
						"data-[active=false]:pointer-events-none data-[active=false]:opacity-0",
					)}
				>
					<ArrowDownIcon className="size-4" />
				</MessageScroller.Button>
				<LoadEarlierOnReachStart
					hasMore={hasMoreMessages}
					isFetching={isFetchingMoreMessages}
					onLoadMore={onFetchMoreMessages}
				/>
			</MessageScroller.Root>
		</MessageScroller.Provider>
	);
};
