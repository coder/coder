import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { type FC } from "react";

interface TerminalRetryConnectionProps {
	/**
	 * Whether a retry is currently in progress
	 */
	isRetrying: boolean;
	/**
	 * Time in milliseconds until the next automatic retry (null if not scheduled)
	 */
	timeUntilNextRetry: number | null;
	/**
	 * Number of retry attempts made
	 */
	attemptCount: number;
	/**
	 * Maximum number of retry attempts
	 */
	maxAttempts: number;
	/**
	 * Callback to manually trigger a retry
	 */
	onRetryNow: () => void;
}

/**
 * Formats milliseconds into a human-readable countdown
 */
function formatCountdown(ms: number): string {
	const seconds = Math.ceil(ms / 1000);
	return `${seconds} second${seconds !== 1 ? "s" : ""}`;
}

export const TerminalRetryConnection: FC<TerminalRetryConnectionProps> = ({
	isRetrying,
	timeUntilNextRetry,
	attemptCount,
	maxAttempts,
	onRetryNow,
}) => {
	// Don't show anything if we're not in a retry state
	if (!isRetrying && timeUntilNextRetry === null && attemptCount === 0) {
		return null;
	}

	// Show different messages based on state
	let message: string;
	let showRetryButton = true;

	if (isRetrying) {
		message = "Reconnecting...";
		showRetryButton = false; // Don't show button while actively retrying
	} else if (timeUntilNextRetry !== null) {
		const countdown = formatCountdown(timeUntilNextRetry);
		message = `Retrying in ${countdown}`;
	} else if (attemptCount >= maxAttempts) {
		message = "Failed after multiple attempts";
	} else {
		message = "";
	}

	return (
		<div className="flex items-center gap-2">
			{message && (
				<span className="text-sm text-content-secondary">{message}</span>
			)}
			{showRetryButton && (
				<Button
					variant="outline"
					size="sm"
					onClick={onRetryNow}
					disabled={isRetrying}
					className="flex items-center gap-1"
				>
					{isRetrying && <Spinner size="sm" />}
					Retry now
				</Button>
			)}
		</div>
	);
};
