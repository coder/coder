import { Alert, type AlertProps } from "components/Alert/Alert";
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

/**
 * Terminal-specific alert component with consistent styling
 */
const TerminalAlert: FC<AlertProps> = (props) => {
	return (
		<Alert
			{...props}
			css={(theme) => ({
				borderRadius: 0,
				borderWidth: 0,
				borderBottomWidth: 1,
				borderBottomColor: theme.palette.divider,
				backgroundColor: theme.palette.background.paper,
				borderLeft: `3px solid ${theme.palette[props.severity!].light}`,
				marginBottom: 1,
			})}
		/>
	);
};

export const TerminalRetryConnection: FC<TerminalRetryConnectionProps> = ({
	isRetrying,
	timeUntilNextRetry,
	attemptCount,
	maxAttempts,
	onRetryNow,
}) => {
	// Don't show anything if we're not in a retry state
	if (!isRetrying && timeUntilNextRetry === null) {
		return null;
	}

	// Show different messages based on state
	let message: string;
	let showRetryButton = true;

	if (isRetrying) {
		message = "Reconnecting to terminal...";
		showRetryButton = false; // Don't show button while actively retrying
	} else if (timeUntilNextRetry !== null) {
		const countdown = formatCountdown(timeUntilNextRetry);
		message = `Connection lost. Retrying in ${countdown}...`;
	} else if (attemptCount >= maxAttempts) {
		message = "Connection failed after multiple attempts.";
	} else {
		message = "Connection lost.";
	}

	return (
		<TerminalAlert
			severity="warning"
			actions={
				showRetryButton ? (
					<Button
						variant="outline"
						size="sm"
						onClick={onRetryNow}
						disabled={isRetrying}
						css={{
							display: "flex",
							alignItems: "center",
							gap: "0.5rem",
						}}
					>
						{isRetrying && <Spinner size="sm" />}
						Retry now
					</Button>
				) : null
			}
		>
			{message}
		</TerminalAlert>
	);
};
