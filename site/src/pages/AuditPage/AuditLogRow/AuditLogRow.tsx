import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import type { AuditLog, BuildReason } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
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
			<TableCell css={styles.auditLogCell}>
				<Stack
					direction="row"
					alignItems="center"
					css={styles.auditLogHeader}
					tabIndex={0}
					onClick={toggle}
					onKeyDown={(event) => {
						if (event.key === "Enter") {
							toggle();
						}
					}}
				>
					<Stack
						direction="row"
						alignItems="center"
						css={styles.auditLogHeaderInfo}
					>
						<Stack direction="row" alignItems="center" css={styles.fullWidth}>
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

							<Stack
								alignItems="baseline"
								css={styles.fullWidth}
								justifyContent="space-between"
								direction="row"
							>
								<Stack
									css={styles.auditLogSummary}
									direction="row"
									alignItems="baseline"
									spacing={1}
								>
									<AuditLogDescription auditLog={auditLog} />
									{auditLog.is_deleted && (
										<span css={styles.deletedLabel}>(deleted)</span>
									)}
									<span css={styles.auditLogTime}>
										{new Date(auditLog.time).toLocaleTimeString()}
									</span>
								</Stack>

								<Stack direction="row" alignItems="center">
									<StatusPill isHttpCode={true} code={auditLog.status_code} />

									{/* With multi-org, there is not enough space so show
                      everything in a tooltip. */}
									{showOrgDetails ? (
										<Tooltip>
											<TooltipTrigger asChild>
												<InfoIcon
													css={(theme) => ({
														color: theme.palette.info.light,
													})}
												/>
											</TooltipTrigger>
											<TooltipContent side="bottom">
												<div css={styles.auditLogInfoTooltip}>
													{auditLog.ip && (
														<div>
															<h4 css={styles.auditLogInfoHeader}>IP:</h4>
															<div>{auditLog.ip}</div>
														</div>
													)}
													{userAgent?.os.name && (
														<div>
															<h4 css={styles.auditLogInfoHeader}>OS:</h4>
															<div>{userAgent.os.name}</div>
														</div>
													)}
													{userAgent?.browser.name && (
														<div>
															<h4 css={styles.auditLogInfoHeader}>Browser:</h4>
															<div>
																{userAgent.browser.name}{" "}
																{userAgent.browser.version}
															</div>
														</div>
													)}
													{auditLog.organization && (
														<div>
															<h4 css={styles.auditLogInfoHeader}>
																Organization:
															</h4>
															<Link
																component={RouterLink}
																to={`/organizations/${auditLog.organization.name}`}
															>
																{auditLog.organization.display_name ||
																	auditLog.organization.name}
															</Link>
														</div>
													)}
													{auditLog.additional_fields?.build_reason &&
														auditLog.action === "start" && (
															<div>
																<h4 css={styles.auditLogInfoHeader}>Reason:</h4>
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
										<Stack direction="row" spacing={1} alignItems="baseline">
											{auditLog.ip && (
												<span css={styles.auditLogInfo}>
													<span>IP: </span>
													<strong>{auditLog.ip}</strong>
												</span>
											)}
											{userAgent?.os.name && (
												<span css={styles.auditLogInfo}>
													<span>OS: </span>
													<strong>{userAgent.os.name}</strong>
												</span>
											)}
											{userAgent?.browser.name && (
												<span css={styles.auditLogInfo}>
													<span>Browser: </span>
													<strong>
														{userAgent.browser.name} {userAgent.browser.version}
													</strong>
												</span>
											)}
											{auditLog.additional_fields?.build_reason &&
												auditLog.action === "start" && (
													<span css={styles.auditLogInfo}>
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
										</Stack>
									)}
								</Stack>
							</Stack>
						</Stack>
					</Stack>

					{shouldDisplayDiff ? (
						<div> {<DropdownArrow close={isDiffOpen} />}</div>
					) : (
						<div css={styles.columnWithoutDiff} />
					)}
				</Stack>

				{shouldDisplayDiff && (
					<Collapse in={isDiffOpen}>
						<AuditLogDiff diff={auditDiff} />
					</Collapse>
				)}
			</TableCell>
		</TimelineEntry>
	);
};

const styles = {
	auditLogCell: {
		padding: "0 !important",
		border: 0,
	},

	auditLogHeader: {
		padding: "16px 32px",
	},

	auditLogHeaderInfo: {
		flex: 1,
	},

	auditLogSummary: (theme) => ({
		...(theme.typography.body1 as CSSObject),
		fontFamily: "inherit",
	}),

	auditLogTime: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 12,
	}),

	auditLogInfo: (theme) => ({
		...(theme.typography.body2 as CSSObject),
		fontSize: 12,
		fontFamily: "inherit",
		color: theme.palette.text.secondary,
		display: "block",
	}),

	auditLogInfoHeader: (theme) => ({
		margin: 0,
		color: theme.palette.text.primary,
		fontSize: 14,
		lineHeight: "150%",
		fontWeight: 600,
	}),

	auditLogInfoTooltip: {
		display: "flex",
		flexDirection: "column",
		gap: 8,
	},

	// offset the absence of the arrow icon on diff-less logs
	columnWithoutDiff: {
		marginLeft: "24px",
	},

	fullWidth: {
		width: "100%",
	},

	deletedLabel: (theme) => ({
		...(theme.typography.caption as CSSObject),
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
