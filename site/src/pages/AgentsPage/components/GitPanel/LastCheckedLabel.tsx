import type { FC } from "react";
import { cn } from "#/utils/cn";
import { relativeTime } from "#/utils/time";

interface LastCheckedLabelProps {
	at: Date | undefined;
	className?: string;
}

/**
 * Renders "checked <relative time>" next to the refresh button so
 * users can tell how stale the local view is. Mirrors the
 * `LastSeen` component pattern: renders on every parent render
 * without a dedicated timer, so the label advances naturally as
 * scan messages arrive (the usual cadence) and on any other
 * re-render. This keeps the label honest ("checked 2 minutes ago"
 * is still accurate if the last scan really was 2 minutes ago) at
 * the cost of sub-second staleness between renders.
 *
 * Returns null when `at` is undefined so the toolbar collapses
 * cleanly before the first scan has been received.
 */
export const LastCheckedLabel: FC<LastCheckedLabelProps> = ({
	at,
	className,
}) => {
	if (!at) {
		return null;
	}
	return (
		<span
			data-testid="git-last-checked"
			className={cn(["whitespace-nowrap", className])}
			title={at.toLocaleString()}
		>
			checked {relativeTime(at)}
		</span>
	);
};
