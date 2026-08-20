import {
	MessageScroller,
	useMessageScrollerScrollable,
} from "@shadcn/react/message-scroller";
import { ArrowDownIcon, RotateCcwIcon } from "lucide-react";
import { type FC, type ReactNode, useEffect } from "react";
import { Button } from "#/components/Button/Button";
import { Spinner } from "#/components/Spinner/Spinner";
import { useStorage } from "#/hooks/useStorage";
import { cn } from "#/utils/cn";
import { chatFullWidthStorage } from "#/utils/storage/keys";
import { chatWidthClass } from "../utils/chatWidth";

interface EarlierMessagesProps {
	hasMoreMessages: boolean;
	isFetchingMoreMessages: boolean;
	// True while fetched pages have not reached the store yet. Blocks paging
	// without showing the loader: hydration is not a history request.
	isHydratingMessages: boolean;
	hasFetchMoreError: boolean;
	hasTranscriptRows: boolean;
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
	isHydratingMessages,
	hasFetchMoreError,
	hasTranscriptRows,
	onFetchMoreMessages,
}) => {
	const { start: canScrollTowardStart } = useMessageScrollerScrollable();

	// "Cannot scroll toward the start" also holds before any row is measured,
	// so only page once rows exist.
	const isAtHistoryStart = !canScrollTowardStart && hasTranscriptRows;
	const shouldLoadEarlierMessages =
		isAtHistoryStart &&
		hasMoreMessages &&
		!isFetchingMoreMessages &&
		!isHydratingMessages &&
		!hasFetchMoreError;

	// The scroller only tells us where the scroll position is through React
	// state. There is no "reached the top" callback to hang this off, so the
	// fetch has to happen in an effect that watches that state. A plain scroll
	// listener cannot replace it: scroll events only fire when there is
	// something to scroll, and the case that needs more history is a
	// transcript shorter than the viewport, where nothing scrolls.
	//
	// The fetch is deferred one frame because the scroller updates its state
	// on an animation frame after new rows appear. Without the wait, a page
	// that just arrived and made the transcript overflow still looks
	// unscrollable for one frame, and we would fetch a page we do not need.
	// If the new page did overflow, the state flips during the wait, this
	// effect re-runs, and the cleanup cancels the fetch before it fires.
	useEffect(() => {
		if (!shouldLoadEarlierMessages) {
			return;
		}
		const frame = requestAnimationFrame(() => {
			void onFetchMoreMessages();
		});
		return () => cancelAnimationFrame(frame);
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
	const [chatFullWidth] = useStorage(chatFullWidthStorage);

	return (
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
	);
};
