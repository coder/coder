import { NetworkIcon } from "lucide-react";
import type {
	ConnectionLog,
	ConnectionLogDuration,
	ConnectionLogDurationStatus,
} from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { StatusBadge } from "#/components/StatusBadge/StatusBadge";
import { TableCell } from "#/components/Table/Table";
import { TimelineEntry } from "#/components/Timeline/TimelineEntry";
import { humanDurationShort } from "#/utils/time";
import { ConnectionLogDescription } from "./ConnectionLogDescription/ConnectionLogDescription";
import { ConnectionLogDetailsPopover } from "./ConnectionLogDetailsPopover";

const unavailableDurationLabels = {
	no_disconnect: "Ongoing",
	missing_connect: "No connect recorded",
	invalid: "Duration unavailable",
} as const satisfies Record<
	Exclude<ConnectionLogDurationStatus, "recorded">,
	string
>;

const durationLabel = ({ status, seconds }: ConnectionLogDuration): string => {
	if (status !== "recorded") {
		return unavailableDurationLabels[status];
	}
	return seconds === undefined
		? unavailableDurationLabels.invalid
		: humanDurationShort(seconds * 1000);
};

type ConnectionLogRowProps = {
	connectionLog: ConnectionLog;
};

export const ConnectionLogRow: React.FC<ConnectionLogRowProps> = ({
	connectionLog,
}) => {
	const isWeb = connectionLog.web_info !== undefined;
	const code =
		connectionLog.web_info?.status_code ?? connectionLog.ssh_info?.exit_code;
	const duration = connectionLog.ssh_info?.duration;
	const durationText = duration ? durationLabel(duration) : undefined;

	return (
		<TimelineEntry
			key={connectionLog.id}
			data-testid={`connection-log-row-${connectionLog.id}`}
			clickable={false}
		>
			<TableCell className="p-0! border-0">
				<div className="flex flex-row items-center gap-4 py-4 px-8">
					<div className="flex flex-row items-center gap-4 flex-1">
						{/* Non-web logs don't have an associated user, so we
						 * display a default network icon instead */}
						{connectionLog.web_info?.user ? (
							<Avatar
								fallback={connectionLog.web_info.user.username}
								src={connectionLog.web_info.user.avatar_url}
							/>
						) : (
							<Avatar>
								<NetworkIcon className="h-full w-full p-1" />
							</Avatar>
						)}

						<div className="flex flex-row items-center justify-between w-full">
							<div className="flex flex-row items-baseline gap-2 text-base">
								<ConnectionLogDescription connectionLog={connectionLog} />
								<span className="text-content-secondary text-xs">
									{new Date(connectionLog.connect_time).toLocaleTimeString()}
									{connectionLog.ssh_info?.disconnect_time &&
										` → ${new Date(connectionLog.ssh_info.disconnect_time).toLocaleTimeString()}`}
								</span>
								{durationText && (
									<span className="text-content-secondary text-xs">
										{durationText}
									</span>
								)}
							</div>

							<div className="flex flex-row items-center gap-4">
								{code !== undefined && (
									<StatusBadge
										code={code}
										isHttpCode={isWeb}
										label={isWeb ? "HTTP Status Code" : "SSH Exit Code"}
									/>
								)}
								<ConnectionLogDetailsPopover connectionLog={connectionLog} />
							</div>
						</div>
					</div>
				</div>
			</TableCell>
		</TimelineEntry>
	);
};
