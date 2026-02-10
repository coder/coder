import type { AuditLog, BuildReason } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	Collapsible,
	CollapsibleContent,
} from "components/Collapsible/Collapsible";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
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
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import userAgentParser from "ua-parser-js";
import { buildReasonLabels } from "utils/workspace";
import { AuditLogDescription } from "./AuditLogDescription/AuditLogDescription";
import { AuditLogDiff } from "./AuditLogDiff/AuditLogDiff";
import {
	determineGroupDiff,
	determineIdPSyncMappingDiff,
} from "./AuditLogDiff/auditUtils";

interface AuditLogRowProps {
	auditLog: AuditLog;
	// Useful for Storybook
	defaultIsDiffOpen?: boolean;
	showOrgDetails: boolean;
}

export const AuditLogRow: FC<AuditLogRowProps> = ({
	auditLog,
	defaultIsDiffOpen = false,
	showOrgDetails,
}) => {
	const [isDiffOpen, setIsDiffOpen] = useState(defaultIsDiffOpen);
	const diffs = Object.entries(auditLog.diff);
	const shouldDisplayDiff = diffs.length > 0;
	const userAgent = auditLog.user_agent
		? userAgentParser(auditLog.user_agent)
		: undefined;

	let auditDiff = auditLog.diff;

	// groups have nested diffs (group members)
	if (auditLog.resource_type === "group") {
		auditDiff = determineGroupDiff(auditLog.diff);
	}

	if (
		auditLog.resource_type === "idp_sync_settings_organization" ||
		auditLog.resource_type === "idp_sync_settings_group" ||
		auditLog.resource_type === "idp_sync_settings_role"
	) {
		auditDiff = determineIdPSyncMappingDiff(auditLog.diff);
	}

	const toggle = () => {
		if (shouldDisplayDiff) {
			setIsDiffOpen((v) => !v);
		}
	};

	return (
		<TimelineEntry
			key={auditLog.id}
			data-testid={`audit-log-row-${auditLog.id}`}
			clickable={shouldDisplayDiff}
		>
			<TableCell className="!p-0 border-0 border-b text-base">
				<Collapsible open={isDiffOpen} onOpenChange={setIsDiffOpen}>
					<div
						className="flex flex-row items-center gap-4 py-4 px-8"
						tabIndex={0}
						role="button"
						onClick={toggle}
						onKeyDown={(event) => {
							if (event.key === "Enter") {
								toggle();
							}
						}}
					>
						<div className="flex flex-row items-center gap-4 flex-1">
							<div className="flex flex-row items-center gap-4 w-full">
								{/*
								 * Session logs don't have an associated user to the log,
								 * so when it happens we display a default icon to represent non user actions
								 */}
								{auditLog.user ? (
									<Avatar
										fallback={auditLog.user.username}
										src={auditLog.user.avatar_url}
									/>
								) : (
									<Avatar>
										<NetworkIcon className="h-full w-full p-1" />
									</Avatar>
								)}

								<div className="flex flex-row items-baseline justify-between w-full font-normal">
									<div className="flex flex-row items-baseline gap-2">
										<AuditLogDescription auditLog={auditLog} />
										{auditLog.is_deleted && (
											<span className="text-xs text-content-secondary">
												(deleted)
											</span>
										)}
										<span className="text-content-secondary text-xs">
											{new Date(auditLog.time).toLocaleTimeString()}
										</span>
									</div>

									<div className="flex flex-row items-center gap-4">
										<StatusPill isHttpCode={true} code={auditLog.status_code} />

										{/* With multi-org, there is not enough space so show
                      everything in a tooltip. */}
										{showOrgDetails ? (
											<Tooltip>
												<TooltipTrigger asChild>
													<InfoIcon className="text-content-link" />
												</TooltipTrigger>
												<TooltipContent side="bottom">
													<div className="flex flex-col gap-2">
														{auditLog.ip && (
															<div>
																<h4 className="m-0 text-content-primary leading-[150%] font-semibold">
																	IP:
																</h4>
																<div>{auditLog.ip}</div>
															</div>
														)}
														{userAgent?.os.name && (
															<div>
																<h4 className="m-0 text-content-primary leading-[150%] font-semibold">
																	OS:
																</h4>
																<div>{userAgent.os.name}</div>
															</div>
														)}
														{userAgent?.browser.name && (
															<div>
																<h4 className="m-0 text-content-primary leading-[150%] font-semibold">
																	Browser:
																</h4>
																<div>
																	{userAgent.browser.name}{" "}
																	{userAgent.browser.version}
																</div>
															</div>
														)}
														{auditLog.organization && (
															<div>
																<h4 className="m-0 text-content-primary leading-[150%] font-semibold">
																	Organization:
																</h4>
																<Link
																	asChild
																	showExternalIcon={false}
																	className="px-0"
																>
																	<RouterLink
																		to={`/organizations/${auditLog.organization.name}`}
																	>
																		{auditLog.organization.display_name ||
																			auditLog.organization.name}
																	</RouterLink>
																</Link>
															</div>
														)}
														{auditLog.additional_fields?.build_reason &&
															auditLog.action === "start" && (
																<div>
																	<h4 className="m-0 text-content-primary leading-normal font-semibold">
																		Reason:
																	</h4>
																	<div>
																		{
																			buildReasonLabels[
																				auditLog.additional_fields
																					.build_reason as BuildReason
																			]
																		}
																	</div>
																</div>
															)}
													</div>
												</TooltipContent>
											</Tooltip>
										) : (
											<div className="flex flex-row items-baseline gap-2">
												{auditLog.ip && (
													<span className="text-xs text-content-secondary block">
														<span>IP: </span>
														<strong>{auditLog.ip}</strong>
													</span>
												)}
												{userAgent?.os.name && (
													<span className="text-xs text-content-secondary block">
														<span>OS: </span>
														<strong>{userAgent.os.name}</strong>
													</span>
												)}
												{userAgent?.browser.name && (
													<span className="text-xs text-content-secondary block">
														<span>Browser: </span>
														<strong>
															{userAgent.browser.name}{" "}
															{userAgent.browser.version}
														</strong>
													</span>
												)}
												{auditLog.additional_fields?.build_reason &&
													auditLog.action === "start" && (
														<span className="text-xs text-content-secondary block">
															<span>Reason: </span>
															<strong>
																{
																	buildReasonLabels[
																		auditLog.additional_fields
																			.build_reason as BuildReason
																	]
																}
															</strong>
														</span>
													)}
											</div>
										)}
									</div>
								</div>
							</div>
						</div>

						{shouldDisplayDiff ? (
							<div>
								<DropdownArrow close={isDiffOpen} />
							</div>
						) : (
							<div className="ml-6" />
						)}
					</div>

					{shouldDisplayDiff && (
						<CollapsibleContent>
							<AuditLogDiff diff={auditDiff} />
						</CollapsibleContent>
					)}
				</Collapsible>
			</TableCell>
		</TimelineEntry>
	);
};
