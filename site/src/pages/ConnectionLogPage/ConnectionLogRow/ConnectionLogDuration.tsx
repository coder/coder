import type {
	ConnectionLog,
	ConnectionLogDurationStatus,
	ConnectionLogSSHInfo,
} from "#/api/typesGenerated";
import {
	HelpPopoverText,
	HelpPopoverTitle,
} from "#/components/HelpPopover/HelpPopover";
import { humanDurationShort } from "#/utils/time";

type CompatibleSSHInfo = Omit<ConnectionLogSSHInfo, "duration_status"> &
	Partial<Pick<ConnectionLogSSHInfo, "duration_status">>;

/** Older replicas can omit duration_status. */
export type CompatibleConnectionLog = Omit<ConnectionLog, "ssh_info"> & {
	ssh_info?: CompatibleSSHInfo;
};

/** Only recorded and unverified intervals have a numeric duration. */
export type ConnectionDuration =
	| { status: "recorded" | "unverified"; seconds: number }
	| { status: Exclude<ConnectionLogDurationStatus, "recorded" | "unverified"> };

const durationStatusLabels: Record<
	Exclude<ConnectionLogDurationStatus, "recorded" | "unverified">,
	string
> = {
	no_disconnect: "No disconnect recorded",
	missing_connect: "No connect recorded",
	invalid: "Duration unavailable",
};

const durationStatusExplanations: Record<ConnectionLogDurationStatus, string> =
	{
		recorded: "Coder recorded both the connect and disconnect events.",
		unverified:
			"The start time is unverified, so the actual duration may differ.",
		no_disconnect: "Duration is unavailable without a disconnect event.",
		missing_connect: "Duration is unavailable without a connect event.",
		invalid:
			"Stored timestamps are missing or out of order, so no duration is shown.",
	};

/** Without duration_status, derives an unverified interval from timestamps. */
export const getConnectionDuration = (
	connectTime: string,
	sshInfo: CompatibleSSHInfo,
): ConnectionDuration => {
	let status = sshInfo.duration_status;
	let seconds = sshInfo.duration_seconds;
	if (status === undefined) {
		if (!sshInfo.disconnect_time) {
			return { status: "no_disconnect" };
		}
		status = "unverified";
		seconds =
			(Date.parse(sshInfo.disconnect_time) - Date.parse(connectTime)) / 1000;
	}
	if (status !== "recorded" && status !== "unverified") {
		return { status };
	}
	return seconds !== undefined && Number.isFinite(seconds) && seconds >= 0
		? { status, seconds }
		: { status: "invalid" };
};

const durationText = (duration: ConnectionDuration): string =>
	"seconds" in duration
		? humanDurationShort(duration.seconds * 1000)
		: durationStatusLabels[duration.status];

type ConnectionLogDurationProps = {
	duration: ConnectionDuration;
};

/** Marks unverified durations inline. */
export const ConnectionLogDurationLabel: React.FC<
	ConnectionLogDurationProps
> = ({ duration }) => (
	<span className="text-content-secondary text-xs">
		{durationText(duration)}
		{duration.status === "unverified" && " (unverified)"}
	</span>
);

/** Popover duration value with its status explanation. */
export const ConnectionLogDurationDetails: React.FC<
	ConnectionLogDurationProps
> = ({ duration }) => (
	<div className="flex flex-col gap-2">
		<div className="flex flex-col gap-1">
			<HelpPopoverTitle className="mb-0">Duration:</HelpPopoverTitle>
			<div
				className={
					"seconds" in duration
						? "text-sm font-normal text-content-primary tabular-nums"
						: "text-xs font-normal text-content-secondary"
				}
			>
				{durationText(duration)}
			</div>
		</div>
		<HelpPopoverText className="m-0 text-xs font-normal leading-relaxed">
			{durationStatusExplanations[duration.status]}
		</HelpPopoverText>
	</div>
);
