import { cn } from "cn";
import { useId } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";

const OVERVIEW_CARD_MAX_HEIGHT = "36rem";
const OVERVIEW_CARD_VIEWPORT_MAX_HEIGHT = "calc(100vh - 6rem)";

export const chatOverviewContainerClassName = "@container/chat";
export const chatOverviewHiddenWhenNarrowClassName = "@max-[72rem]/chat:hidden";

/** Places the card below the chat top bar, at the chat's right edge. */
export const chatOverviewCardPositionClassName = cn(
	"absolute top-full right-4 z-20 mt-2",
	chatOverviewHiddenWhenNarrowClassName,
);

/**
 * Apply while the card is open. Sets `--chat-overview-reserve`, the right
 * padding the transcript and composer leave so the card does not cover them.
 * From 84rem the centered transcript already clears the card.
 */
export const chatOverviewReserveClassName =
	"@min-[72rem]/chat:@max-[84rem]/chat:[--chat-overview-reserve:18rem]";

type ChatOverviewCardShellProps = React.PropsWithChildren<{
	id?: string;
	isLoading: boolean;
	className?: string;
}>;

/**
 * The overview card's frame and heading. It has no chat dependencies, so the
 * route skeleton, which the router imports eagerly, can render it too.
 */
export const ChatOverviewCardShell: React.FC<ChatOverviewCardShellProps> = ({
	id,
	isLoading,
	className,
	children,
}) => {
	const headingId = useId();
	return (
		<aside
			id={id}
			aria-labelledby={headingId}
			aria-busy={isLoading}
			className={cn(
				"flex w-64 flex-col divide-y divide-solid divide-border overflow-y-auto rounded-lg border border-solid border-border bg-surface-primary px-1.5 text-sm shadow-md",
				className,
			)}
			style={{
				maxHeight: `min(${OVERVIEW_CARD_MAX_HEIGHT}, ${OVERVIEW_CARD_VIEWPORT_MAX_HEIGHT})`,
			}}
		>
			<h2
				id={headingId}
				className="m-0 px-1.5 py-2 text-xs font-medium text-content-secondary"
			>
				Overview
			</h2>
			{children}
		</aside>
	);
};

/** Placeholder rows while the card has no data to show yet. */
export const ChatOverviewCardLoadingRows: React.FC = () => (
	<div role="status" className="flex flex-col gap-2 px-1.5 py-2">
		<span className="sr-only">Loading overview</span>
		{[0, 1, 2].map((row) => (
			<div key={row} aria-hidden className="flex items-center gap-2">
				<Skeleton variant="circular" className="size-4 shrink-0" />
				<Skeleton variant="text" className="flex-1" />
			</div>
		))}
	</div>
);
