import type { ConnectionLog } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Link } from "components/Link/Link";
import { StatusPill } from "components/StatusPill/StatusPill";
import { TableCell } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon, NetworkIcon } from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import userAgentParser from "ua-parser-js";
import { connectionTypeIsWeb } from "utils/connection";
import { ConnectionLogDescription } from "./ConnectionLogDescription/ConnectionLogDescription";

interface ConnectionLogRowProps {
	connectionLog: ConnectionLog;
}

export const ConnectionLogRow: FC<ConnectionLogRowProps> = ({
	connectionLog,
}) => {
	const userAgent = connectionLog.web_info?.user_agent
		? userAgentParser(connectionLog.web_info?.user_agent)
		: undefined;
	const isWeb = connectionTypeIsWeb(connectionLog.type);
	const code =
		connectionLog.web_info?.status_code ?? connectionLog.ssh_info?.exit_code;

	return (
		<TimelineEntry
			key={connectionLog.id}
			data-testid={`connection-log-row-${connectionLog.id}`}
			clickable={false}
		>
			<TableCell className="!p-0 border-0">
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
							</div>

							<div className="flex flex-row items-center gap-4">
								{code !== undefined && (
									<StatusPill
										code={code}
										isHttpCode={isWeb}
										label={isWeb ? "HTTP Status Code" : "SSH Exit Code"}
									/>
								)}
								<Tooltip>
									<TooltipTrigger asChild>
										<InfoIcon className="text-content-link" />
									</TooltipTrigger>
									<TooltipContent side="bottom">
										<div className="flex flex-col gap-2">
											{connectionLog.ip && (
												<div>
													<h4 className="m-0 text-content-primary text-sm leading-[150%] font-semibold">
														IP:
													</h4>
													<div>{connectionLog.ip}</div>
												</div>
											)}
											{userAgent?.os.name && (
												<div>
													<h4 className="m-0 text-content-primary text-sm leading-[150%] font-semibold">
														OS:
													</h4>
													<div>{userAgent.os.name}</div>
												</div>
											)}
											{userAgent?.browser.name && (
												<div>
													<h4 className="m-0 text-content-primary text-sm leading-[150%] font-semibold">
														Browser:
													</h4>
													<div>
														{userAgent.browser.name} {userAgent.browser.version}
													</div>
												</div>
											)}
											{connectionLog.organization && (
												<div>
													<h4 className="m-0 text-content-primary text-sm leading-[150%] font-semibold">
														Organization:
													</h4>
													<Link
														asChild
														showExternalIcon={false}
														className="px-0 text-xs"
													>
														<RouterLink
															to={`/organizations/${connectionLog.organization.name}`}
														>
															{connectionLog.organization.display_name ||
																connectionLog.organization.name}
														</RouterLink>
													</Link>
												</div>
											)}
											{connectionLog.ssh_info?.disconnect_reason && (
												<div>
													<h4 className="m-0 text-content-primary text-sm leading-[150%] font-semibold">
														Close Reason:
													</h4>
													<div>{connectionLog.ssh_info?.disconnect_reason}</div>
												</div>
											)}
										</div>
									</TooltipContent>
								</Tooltip>
							</div>
						</div>
					</div>
				</div>
			</TableCell>
		</TimelineEntry>
	);
};
