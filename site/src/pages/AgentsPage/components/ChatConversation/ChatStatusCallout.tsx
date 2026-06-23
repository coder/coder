import { type FC, useEffect, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "#/components/Alert/Alert";
import { Link } from "#/components/Link/Link";
import { getProviderStatusURL } from "./chatStatusHelpers";
import type { LiveStatusModel } from "./liveStatusModel";

type RetryOrFailedStatus = Extract<
	LiveStatusModel,
	{ phase: "retrying" } | { phase: "failed" }
>;
type ReconnectingStatus = Extract<LiveStatusModel, { phase: "reconnecting" }>;

/**
 * Syncs with the system clock to produce a live countdown from an
 * ISO-8601 deadline. Polls at 100ms so the displayed second flips
 * within 100ms of the real transition. Returns 0 when no deadline is
 * provided or the deadline has passed.
 */
const useDeadlineCountdown = (deadline: string | undefined): number => {
	const [secondsLeft, setSecondsLeft] = useState(0);

	useEffect(() => {
		if (!deadline) {
			setSecondsLeft(0);
			return;
		}

		const targetMs = new Date(deadline).getTime();
		if (!Number.isFinite(targetMs)) {
			setSecondsLeft(0);
			return;
		}

		const update = () => {
			const remaining = Math.max(0, targetMs - Date.now());
			setSecondsLeft(Math.ceil(remaining / 1000));
		};

		update();
		const interval = setInterval(update, 100);
		return () => clearInterval(interval);
	}, [deadline]);

	return secondsLeft;
};

/**
 * Leaf component that owns the countdown interval so ticking seconds only
 * re-render this span, not the parent Alert (which contains a Radix Slot
 * that infinite-loops on rapid re-renders).
 */
const StatusCountdown: FC<{
	deadline: string;
	label: string;
}> = ({ deadline, label }) => {
	const seconds = useDeadlineCountdown(deadline);
	if (seconds <= 0) {
		return null;
	}
	return (
		<span>
			{label} {seconds}s
		</span>
	);
};

const StatusAlert: FC<{ status: RetryOrFailedStatus }> = ({ status }) => {
	const statusURL = getProviderStatusURL(status.kind, status.provider);
	const severity =
		status.phase === "failed"
			? "error"
			: status.kind === "generic"
				? "info"
				: "warning";
	const metadataItems: React.ReactNode[] = [];
	if (status.phase === "retrying" && status.retryingAt) {
		metadataItems.push(
			<StatusCountdown
				key="countdown"
				deadline={status.retryingAt}
				label="Retrying in"
			/>,
		);
	}
	if (status.phase === "retrying") {
		metadataItems.push(<span key="attempt">Attempt {status.attempt}</span>);
	}
	if (status.phase === "failed" && status.statusCode != null) {
		metadataItems.push(<span key="code">HTTP {status.statusCode}</span>);
	}

	return (
		<Alert
			severity={severity}
			actions={
				metadataItems.length > 0 ? (
					<div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
						{metadataItems}
					</div>
				) : undefined
			}
		>
			<AlertTitle>{status.title}</AlertTitle>
			<AlertDescription>
				<span>
					{status.message}{" "}
					{statusURL && (
						<Link href={statusURL} target="_blank" rel="noreferrer">
							Status
						</Link>
					)}
				</span>
				{status.phase === "failed" &&
					status.detail &&
					(status.kind === "generic" ? (
						<code className="mt-1 block whitespace-pre-wrap text-xs text-content-secondary font-mono bg-surface-secondary rounded-md">
							{status.detail}
						</code>
					) : (
						<span className="mt-1 block text-content-secondary">
							{status.detail}
						</span>
					))}
			</AlertDescription>
		</Alert>
	);
};

const ReconnectingAlert: FC<{ status: ReconnectingStatus }> = ({ status }) => {
	return (
		<Alert
			severity="info"
			actions={
				<div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
					<StatusCountdown
						deadline={status.retryingAt}
						label="Reconnecting in"
					/>
					<span>Attempt {status.attempt}</span>
				</div>
			}
		>
			<AlertTitle>{status.title}</AlertTitle>
			<AlertDescription>{status.message}</AlertDescription>
		</Alert>
	);
};

export const ChatStatusCallout: FC<{
	status: LiveStatusModel;
}> = ({ status }) => {
	switch (status.phase) {
		case "idle":
		case "streaming":
		case "starting":
			return null;
		case "retrying":
			return <StatusAlert status={status} />;
		case "reconnecting":
			return <ReconnectingAlert status={status} />;
		case "failed":
			return <StatusAlert status={status} />;
	}
};
