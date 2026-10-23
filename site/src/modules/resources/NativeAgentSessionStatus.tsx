import type { SessionInfo } from "@agentclientprotocol/sdk";
import { Alert, AlertTitle } from "#/components/Alert/Alert";
import { Badge } from "#/components/Badge/Badge";
import { useTime } from "#/hooks/useTime";
import { timeFrom } from "#/utils/time";
import { nativeAgentMetadata } from "./nativeAgentMetadata";

type NativeAgentTimestampProps = {
	timestamp: number | undefined;
	prefix: "Started" | "Last error";
	now?: number;
};

/** Updates only the relative label, not the session list or streaming transcript. */
export const NativeAgentTimestamp: React.FC<NativeAgentTimestampProps> = ({
	timestamp,
	prefix,
	now,
}) => {
	const clock = useTime(Date.now, {
		interval: 30_000,
		disabled: now !== undefined || timestamp === undefined,
	});
	if (timestamp === undefined) {
		return (
			<span>
				{prefix === "Started" ? "Unknown start time" : "Unknown error time"}
			</span>
		);
	}
	const exact = new Date(timestamp).toISOString();
	return (
		<time dateTime={exact} title={`${prefix}: ${exact}`}>
			{prefix} {timeFrom(timestamp, now ?? clock)}
		</time>
	);
};

type NativeAgentSessionStatusProps = { info: SessionInfo };

/** Distinguishes integrity faults and ended sessions from heartbeat availability. */
export const NativeAgentSessionStatus: React.FC<
	NativeAgentSessionStatusProps
> = ({ info }) => {
	const { fault, ended, stale, lastError, registrationPending, blockedReason } =
		nativeAgentMetadata(info);
	return (
		<span className="flex flex-wrap gap-1">
			{registrationPending && !fault && !ended && (
				<Badge
					asChild
					size="xs"
					variant={blockedReason ? "warning" : "default"}
				>
					<span>{blockedReason ? "Needs attention" : "Starting"}</span>
				</Badge>
			)}
			{fault && (
				<Badge asChild size="xs" variant="destructive">
					<span>Faulted</span>
				</Badge>
			)}
			{ended && (
				<Badge asChild size="xs">
					<span>Ended</span>
				</Badge>
			)}
			{stale && (
				<Badge asChild size="xs" variant="warning">
					<span>Heartbeat stale</span>
				</Badge>
			)}
			{lastError && !fault && (
				<Badge asChild size="xs" variant="warning">
					<span>Last error recorded</span>
				</Badge>
			)}
		</span>
	);
};

type NativeAgentSessionErrorProps = { info: SessionInfo; now?: number };

/** Keeps the broker's latest error inspectable even if loading history fails. */
export const NativeAgentSessionError: React.FC<
	NativeAgentSessionErrorProps
> = ({ info, now }) => {
	const { fault, ended, stale, lastError } = nativeAgentMetadata(info);
	return (
		<>
			{stale && !fault && !ended && (
				<Alert severity="warning">
					Heartbeat is stale. Availability is unknown, not a confirmed fault.
					Refresh sessions to check again before joining or prompting.
				</Alert>
			)}
			{(fault || ended || lastError) && (
				<Alert severity={fault ? "error" : lastError ? "warning" : "info"}>
					<AlertTitle>
						{fault
							? "Session fault"
							: ended
								? "Session ended"
								: "Last reported error"}
					</AlertTitle>
					{lastError && (
						<>
							<p className="my-2 whitespace-pre-wrap wrap-anywhere">
								{lastError.message}
							</p>
							<p className="my-2 text-xs text-content-secondary">
								<NativeAgentTimestamp
									timestamp={lastError.at}
									prefix="Last error"
									now={now}
								/>
							</p>
						</>
					)}
					<p className="mb-0 mt-2 text-content-secondary">
						{fault
							? "Session integrity is not verified. Joining and prompting are unavailable. Retained details remain available for inspection."
							: ended
								? "This session has ended. Joining and prompting are unavailable."
								: "This is the latest reported failure, not a session fault. It remains visible after recovery."}
					</p>
				</Alert>
			)}
		</>
	);
};
