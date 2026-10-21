import { NetworkIcon } from "lucide-react";
import type { ConnectionLog } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { StatusBadge } from "#/components/StatusBadge/StatusBadge";
import { TableCell } from "#/components/Table/Table";
import { TimelineEntry } from "#/components/Timeline/TimelineEntry";
import { humanDurationShort } from "#/utils/time";
import { ConnectionLogDescription } from "./ConnectionLogDescription/ConnectionLogDescription";
import { ConnectionLogDetailsPopover } from "./ConnectionLogDetailsPopover";

type ConnectionTiming = {
	range: string;
	detail?: string;
};

const formatTime = (time: string): string =>
	new Date(time).toLocaleTimeString();

const connectionTiming = ({
	connect_time,
	ssh_info,
}: ConnectionLog): ConnectionTiming => {
	const start = formatTime(connect_time);
	const end = ssh_info?.disconnect_time && formatTime(ssh_info.disconnect_time);
	switch (ssh_info?.duration?.status) {
		case "no_disconnect":
			return { range: `${start} → Ongoing` };
		case "missing_connect":
			return { range: `Unknown → ${end}` };
		case "invalid":
			return {
				range: `${start} → ${end}`,
				detail: "Disconnect recorded before connect",
			};
		case "recorded": {
			const seconds = ssh_info.duration.seconds;
			return {
				range: `${start} → ${end}`,
				detail:
					seconds === undefined
						? undefined
						: humanDurationShort(seconds * 1000),
			};
		}
		default:
			return { range: end ? `${start} → ${end}` : start };
	}
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
	const timing = connectionTiming(connectionLog);

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
									{timing.range}
								</span>
								{timing.detail && (
									<span className="text-content-secondary text-xs">
										{timing.detail}
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
