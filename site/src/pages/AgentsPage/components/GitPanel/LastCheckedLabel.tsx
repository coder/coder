import type { FC } from "react";
import { cn } from "#/utils/cn";
import { relativeTime } from "#/utils/time";

interface LastCheckedLabelProps {
	at: Date | undefined;
	className?: string;
}

/**
 * Renders "checked <relative time>" next to the refresh button.
 * Compiled by React Compiler, so re-renders are driven by a fresh
 * `at` reference; the server's 5s scan heartbeat supplies that.
 * Returns null before the first scan so the toolbar collapses.
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
