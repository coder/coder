import { ArrowDownIcon } from "lucide-react";
import type { FC, RefObject } from "react";
import { Button } from "#/components/Button/Button";
import { cn } from "#/utils/cn";
import { useScrollAnchoring } from "./useScrollAnchoring";

interface ScrollAnchoredContainerProps {
	scrollContainerRef: RefObject<HTMLDivElement | null>;
	scrollToBottomRef: RefObject<(() => void) | null>;
	isFetchingMoreMessages: boolean;
	hasMoreMessages: boolean;
	onFetchMoreMessages: () => void;
	children: React.ReactNode;
}

/**
 * Scroll container that keeps the transcript in normal top-to-bottom
 * document flow while preserving a bottom-anchored chat experience.
 * The user is at the bottom when the remaining scroll distance to the
 * end of the container is within SCROLL_THRESHOLD.
 *
 * Handles:
 * - Loading older message pages via an IntersectionObserver sentinel.
 * - ResizeObserver-driven scroll anchoring for transcript and viewport
 *   size changes.
 * - A floating "Scroll to bottom" button when the user is scrolled
 *   away from the bottom.
 *
 * CSS overflow anchoring is disabled on the container, so all position
 * restoration is done manually.
 */
export const ScrollAnchoredContainer: FC<ScrollAnchoredContainerProps> = ({
	scrollContainerRef,
	scrollToBottomRef,
	isFetchingMoreMessages,
	hasMoreMessages,
	onFetchMoreMessages,
	children,
}) => {
	const { contentRef, sentinelRef, showScrollToBottom, handleScrollToBottom } =
		useScrollAnchoring({
			scrollContainerRef,
			scrollToBottomRef,
			isFetchingMoreMessages,
			onFetchMoreMessages,
		});

	return (
		<div className="relative flex min-h-0 flex-1 flex-col">
			<div
				ref={scrollContainerRef}
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
						showScrollToBottom
							? "pointer-events-auto translate-y-0 opacity-100"
							: "translate-y-2 opacity-0",
					)}
					onClick={handleScrollToBottom}
					aria-label="Scroll to bottom"
					aria-hidden={!showScrollToBottom || undefined}
					tabIndex={showScrollToBottom ? undefined : -1}
				>
					<ArrowDownIcon />
				</Button>
			</div>
		</div>
	);
};
