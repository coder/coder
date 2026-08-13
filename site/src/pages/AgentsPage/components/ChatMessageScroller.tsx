import {
	MessageScroller,
	useMessageScrollerScrollable,
	useMessageScrollerVisibility,
} from "@shadcn/react/message-scroller";
import { ArrowDownIcon, RotateCcwIcon } from "lucide-react";
import { type FC, type ReactNode, useEffect } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { cn } from "#/utils/cn";
import { chatWidthClass, useChatFullWidth } from "../hooks/useChatFullWidth";

interface EarlierMessagesProps {
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	hasFetchMoreError: boolean;
	onFetchMoreMessages: () => Promise<unknown>;
}

/**
 * Owns history paging for the transcript. It reads the scroller's own state
 * instead of measuring the viewport, and it never moves the scroll position:
 * MessageScroller keeps the reading position across a prepend on its own.
 */
const EarlierMessages: FC<EarlierMessagesProps> = ({
	hasMoreMessages,
	isFetchingMoreMessages,
	hasFetchMoreError,
	onFetchMoreMessages,
}) => {
	const { start: canScrollTowardStart } = useMessageScrollerScrollable();
	const { visibleMessageIds } = useMessageScrollerVisibility();

	// The scroller reports that it cannot scroll toward the start both before it
	// has measured anything and once the viewport sits at the start edge, so wait
	// for a measured row before reading that as a request for older history. A
	// transcript that does not fill the viewport stays at the start edge, which
	// keeps paging until it overflows or history runs out.
	const isAtHistoryStart =
		!canScrollTowardStart && visibleMessageIds.length > 0;
	const shouldLoadEarlierMessages =
		isAtHistoryStart &&
		hasMoreMessages &&
		!isFetchingMoreMessages &&
		!hasFetchMoreError;

	// isFetchingMoreMessages (from React Query) is the only fence: the effect
	// does not re-fire while a page request is already running.
	useEffect(() => {
		if (shouldLoadEarlierMessages) {
			void onFetchMoreMessages();
		}
	}, [shouldLoadEarlierMessages, onFetchMoreMessages]);

	if (isFetchingMoreMessages) {
		return (
			<div
				role="status"
				aria-label="Loading earlier messages"
				className="pointer-events-none absolute inset-x-0 top-2 z-10 flex justify-center"
			>
				<div className="flex items-center gap-2 rounded-full border border-border-default bg-surface-primary px-3 py-1.5 text-xs text-content-secondary shadow-sm">
					<Spinner className="size-4" loading aria-hidden />
					Loading earlier messages
				</div>
			</div>
		);
	}

	if (!hasFetchMoreError) {
		return null;
	}

	return (
		<div className="absolute inset-x-0 top-2 z-10 flex justify-center">
			<Button
				variant="outline"
				size="sm"
				className="bg-surface-primary shadow-sm"
				onClick={() => void onFetchMoreMessages()}
			>
				<RotateCcwIcon />
				Retry loading earlier messages
			</Button>
		</div>
	);
};

interface ChatMessageScrollerProps extends EarlierMessagesProps {
	/** One `MessageScroller.Item` per transcript row, and nothing else. */
	children: ReactNode;
}

export const ChatMessageScroller: FC<ChatMessageScrollerProps> = ({
	children,
	...earlierMessages
}) => {
	const [chatFullWidth] = useChatFullWidth();

	return (
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<MessageScroller.Root className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
				<MessageScroller.Viewport className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain [scrollbar-gutter:stable] [scrollbar-width:thin] [scrollbar-color:hsl(var(--surface-quaternary))_transparent]">
					<MessageScroller.Content
						data-testid="conversation-timeline"
						aria-busy={earlierMessages.isFetchingMoreMessages || undefined}
						className={cn(
							"mx-auto flex w-full flex-col gap-2 px-4 py-6",
							chatWidthClass(chatFullWidth),
						)}
					>
						{children}
					</MessageScroller.Content>
				</MessageScroller.Viewport>

				<MessageScroller.Button
					direction="end"
					aria-label="Scroll to bottom"
					render={
						<Button
							variant="outline"
							size="icon"
							className="absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-full bg-surface-primary shadow-md transition-all duration-200 data-[active=false]:translate-y-2 data-[active=false]:opacity-0"
						/>
					}
				>
					<ArrowDownIcon />
				</MessageScroller.Button>

				<EarlierMessages {...earlierMessages} />
			</MessageScroller.Root>
		</MessageScroller.Provider>
	);
};
