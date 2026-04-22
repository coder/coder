import { type FC, useEffect, useState } from "react";

/**
 * Formats a compact "checked Ns ago" style label for a past timestamp.
 * Resolution is 1s up to a minute, then 1m up to an hour, then 1h up
 * to a day. Older than that falls back to a locale date string.
 *
 * Exported so tests can pin the exact format without mounting React.
 */
export function checkedAgoLabel(at: Date, now: Date = new Date()): string {
	const diffMs = now.getTime() - at.getTime();
	if (diffMs < 0 || diffMs < 2_000) {
		return "checked just now";
	}
	const seconds = Math.floor(diffMs / 1_000);
	if (seconds < 60) {
		return `checked ${seconds}s ago`;
	}
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) {
		return `checked ${minutes}m ago`;
	}
	const hours = Math.floor(minutes / 60);
	if (hours < 24) {
		return `checked ${hours}h ago`;
	}
	return `checked ${at.toLocaleDateString()}`;
}

interface LastCheckedLabelProps {
	at: Date | undefined;
	className?: string;
}

/**
 * Renders a live-updating "checked Ns ago" label for the given
 * timestamp. Returns null when `at` is undefined so the toolbar
 * collapses cleanly before the first scan has been received.
 *
 * The ticker is scoped to the component's mount lifetime; unmounting
 * the parent (e.g. closing the agent chat) stops it.
 */
export const LastCheckedLabel: FC<LastCheckedLabelProps> = ({
	at,
	className,
}) => {
	// Re-render once per second while mounted so the label advances
	// without requiring the hook to push updates.
	const [, setTick] = useState(0);
	useEffect(() => {
		if (!at) {
			return;
		}
		const id = setInterval(() => {
			setTick((n) => n + 1);
		}, 1_000);
		return () => clearInterval(id);
	}, [at]);

	if (!at) {
		return null;
	}
	return (
		<span
			data-testid="git-last-checked"
			className={className}
			title={at.toLocaleString()}
		>
			{checkedAgoLabel(at)}
		</span>
	);
};
