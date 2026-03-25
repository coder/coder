import { Alert, AlertDescription, AlertTitle } from "components/Alert/Alert";
import { Response, Shimmer } from "components/ai-elements";
import { Button } from "components/Button/Button";
import { Pill } from "components/Pill/Pill";
import { ExternalLinkIcon } from "lucide-react";
import { type FC, useEffect, useState } from "react";
import { getProviderStatusURL } from "./chatStatusHelpers";
import type { LiveStatusModel } from "./liveStatusModel";

const RESPONSE_STARTUP_GRACE_MS = 15_000;
const DELAYED_STARTUP_TEXT = "Response startup is taking longer than expected";
const THINKING_TEXT = "Thinking...";

type RetryOrFailedStatus = Extract<
	LiveStatusModel,
	{ phase: "retrying" } | { phase: "failed" }
>;
type ReconnectingStatus = Extract<LiveStatusModel, { phase: "reconnecting" }>;

const StatusPlaceholder: FC<{
	text: string;
	shimmer?: boolean;
}> = ({ text, shimmer = false }) => {
	return (
		<div className="relative">
			{/* Reserve the final response height without exposing a selectable copy. */}
			<Response aria-hidden className="invisible select-none">
				{text}
			</Response>
			<div className="pointer-events-none absolute inset-0 flex items-baseline gap-2">
				{shimmer ? (
					<Shimmer as="div" className="text-[13px] leading-relaxed">
						{text}
					</Shimmer>
				) : (
					<span className="text-[13px] leading-relaxed text-content-secondary">
						{text}
					</span>
				)}
			</div>
		</div>
	);
};

const StartingPlaceholder: FC = () => {
	const [isDelayed, setIsDelayed] = useState(false);

	useEffect(() => {
		const timeout = window.setTimeout(() => {
			setIsDelayed(true);
		}, RESPONSE_STARTUP_GRACE_MS);
		return () => window.clearTimeout(timeout);
	}, []);

	return (
		<StatusPlaceholder
			text={isDelayed ? DELAYED_STARTUP_TEXT : THINKING_TEXT}
			shimmer={!isDelayed}
		/>
	);
};

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
	const pillType =
		status.phase === "failed"
			? "error"
			: status.kind === "generic"
				? "inactive"
				: "warning";
	const severity =
		status.phase === "failed"
			? "error"
			: status.kind === "generic"
				? "info"
				: "warning";
	const hasMetadata =
		status.phase === "retrying" ||
		status.provider !== undefined ||
		(status.phase === "failed" && status.statusCode !== undefined) ||
		(status.phase === "failed" && status.retryable !== undefined);

	return (
		<Alert
			severity={severity}
			className="py-3"
			actions={
				statusURL && (
					<Button asChild variant="subtle" size="sm">
						<a href={statusURL} target="_blank" rel="noreferrer">
							Status
							<ExternalLinkIcon />
						</a>
					</Button>
				)
			}
		>
			<div className="space-y-2.5">
				<div className="flex flex-wrap items-center gap-2">
					<AlertTitle>{status.title}</AlertTitle>
					<Pill
						className="h-5 px-2.5 text-[10px] font-semibold"
						type={pillType}
					>
						{status.kind}
					</Pill>
				</div>
				<AlertDescription>{status.message}</AlertDescription>
				{hasMetadata && (
					<div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
						{status.phase === "retrying" && status.retryingAt && (
							<StatusCountdown
								deadline={status.retryingAt}
								label="Retrying in"
							/>
						)}
						{status.phase === "retrying" && (
							<span>Attempt {status.attempt}</span>
						)}
						{status.provider && <span>Provider {status.provider}</span>}
						{status.phase === "failed" && status.statusCode !== undefined && (
							<span>HTTP {status.statusCode}</span>
						)}
						{status.phase === "failed" && status.retryable !== undefined && (
							<span>{status.retryable ? "Retryable" : "Not retryable"}</span>
						)}
					</div>
				)}
			</div>
		</Alert>
	);
};

const ReconnectingAlert: FC<{ status: ReconnectingStatus }> = ({ status }) => {
	return (
		<Alert severity="info" className="py-3">
			<div className="space-y-2.5">
				<AlertTitle>{status.title}</AlertTitle>
				<AlertDescription>{status.message}</AlertDescription>
				<div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-content-secondary">
					<StatusCountdown
						deadline={status.retryingAt}
						label="Reconnecting in"
					/>
					<span>Attempt {status.attempt}</span>
				</div>
			</div>
		</Alert>
	);
};

export const ChatStatusCallout: FC<{
	status: LiveStatusModel;
	startingResetKey?: string;
}> = ({ status, startingResetKey }) => {
	switch (status.phase) {
		case "idle":
		case "streaming":
			return null;
		case "starting":
			return <StartingPlaceholder key={startingResetKey ?? "starting"} />;
		case "retrying":
			return (
				<>
					<StatusAlert status={status} />
					<StatusPlaceholder text={THINKING_TEXT} shimmer />
				</>
			);
		case "reconnecting":
			return (
				<>
					<ReconnectingAlert status={status} />
					<StatusPlaceholder text={THINKING_TEXT} shimmer />
				</>
			);
		case "failed":
			return <StatusAlert status={status} />;
	}
};
