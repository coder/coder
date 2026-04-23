import type { FC } from "react";
import { cn } from "#/utils/cn";
import { relativeTime } from "#/utils/time";

interface LastCheckedLabelProps {
	at: Date | undefined;
	className?: string;
}

/**
 * Renders "checked <relative time>" next to the refresh button so
 * users can tell how stale the local view is.
 *
 * The server emits a heartbeat message on every scan (see
 * `agent/agentgit/agentgit.go` `Scan`), so `at` receives a fresh
 * `Date` every `fallbackPollInterval` (5s). That fresh reference is
 * what drives this component's re-renders: the React Compiler
 * memoizes this file (see `site/vite.config.mts`) and keys the
 * cached element on the `at` prop, so a parent re-render that does
 * not change `at` will not refresh the label text. Heartbeat-driven
 * prop changes invalidate the cache and run `relativeTime(at)`
 * again.
 *
 * Staleness between heartbeats is bounded by the scan interval
 * (~5s). At dayjs' relative-time resolution ("a few seconds ago"
 * covers 0-44s) that is imperceptible.
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
