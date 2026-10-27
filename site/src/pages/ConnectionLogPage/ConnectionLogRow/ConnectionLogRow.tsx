import { NetworkIcon, TimerIcon } from "lucide-react";
import type { ConnectionLog } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Badge } from "#/components/Badge/Badge";
import { StatusBadge } from "#/components/StatusBadge/StatusBadge";
import { TableCell } from "#/components/Table/Table";
import { TimelineEntry } from "#/components/Timeline/TimelineEntry";
import { humanDurationShort } from "#/utils/time";
import { ConnectionLogDescription } from "./ConnectionLogDescription/ConnectionLogDescription";
import { ConnectionLogDetailsPopover } from "./ConnectionLogDetailsPopover";

type ConnectionTiming = {
	range: string;
	duration?: string;
	warning?: string;
};

const formatTime = (time: string): string =>
	new Date(time).toLocaleTimeString();

const connectionTiming = ({
	connect_time,
	ssh_info,
}: ConnectionLog): ConnectionTiming => {
	const start = formatTime(connect_time);
	if (!ssh_info) {
		return { range: start };
	}
	const disconnectTime = ssh_info.disconnect_time;
	if (!disconnectTime) {
		return { range: `${start} → Ongoing` };
	}
	const end = formatTime(disconnectTime);
	// A disconnect recorded without its connect stores its own time as
	// connect_time, so identical timestamps mean the connect is unknown.
	if (connect_time === disconnectTime) {
		return { range: `Unknown → ${end}` };
	}
	const durationMs =
		new Date(disconnectTime).getTime() - new Date(connect_time).getTime();
	if (durationMs < 0) {
		return {
			range: `${start} → ${end}`,
			warning: "Disconnect recorded before connect",
		};
	}
	return {
		range: `${start} → ${end}`,
		duration: humanDurationShort(durationMs),
	};
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
							<div className="flex flex-row items-center gap-2 text-base">
								<ConnectionLogDescription connectionLog={connectionLog} />
								<div className="flex flex-row items-center gap-1">
									<Badge size="sm">{timing.range}</Badge>
									{timing.duration && (
										<Badge size="sm">
											<TimerIcon />
											{timing.duration}
										</Badge>
									)}
									{timing.warning && (
										<Badge size="sm" variant="warning">
											{timing.warning}
										</Badge>
									)}
								</div>
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
