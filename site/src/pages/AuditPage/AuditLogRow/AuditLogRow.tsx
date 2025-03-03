import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import InfoOutlined from "@mui/icons-material/InfoOutlined";
import Collapse from "@mui/material/Collapse";
import Link from "@mui/material/Link";
import TableCell from "@mui/material/TableCell";
import Tooltip from "@mui/material/Tooltip";
import type { AuditLog } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Pill } from "components/Pill/Pill";
import { Stack } from "components/Stack/Stack";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { ThemeRole } from "theme/roles";
import userAgentParser from "ua-parser-js";
import { AuditLogDescription } from "./AuditLogDescription/AuditLogDescription";
import { AuditLogDiff } from "./AuditLogDiff/AuditLogDiff";
import {
	determineGroupDiff,
	determineIdPSyncMappingDiff,
} from "./AuditLogDiff/auditUtils";
import { NetworkIcon } from "lucide-react";

const httpStatusColor = (httpStatus: number): ThemeRole => {
	// Treat server errors (500) as errors
	if (httpStatus >= 500) {
		return "error";
	}

	// Treat client errors (400) as warnings
	if (httpStatus >= 400) {
		return "warning";
	}

	// OK (200) and redirects (300) are successful
	return "success";
};

export interface AuditLogRowProps {
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
									<StatusPill code={auditLog.status_code} />

									{/* With multi-org, there is not enough space so show
                      everything in a tooltip. */}
									{showOrgDetails ? (
										<Tooltip
											title={
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
													{auditLog.additional_fields?.reason && (
														<div>
															<h4 css={styles.auditLogInfoHeader}>Reason:</h4>
															<div>{auditLog.additional_fields?.reason}</div>
														</div>
													)}
												</div>
											}
										>
											<InfoOutlined
												css={(theme) => ({
													fontSize: 20,
													color: theme.palette.info.light,
												})}
											/>
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

function StatusPill({ code }: { code: number }) {
	const isHttp = code >= 100;

	return (
		<Pill
			css={styles.statusCodePill}
			type={isHttp ? httpStatusColor(code) : code === 0 ? "success" : "error"}
		>
			{code.toString()}
		</Pill>
	);
}

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

	statusCodePill: {
		fontSize: 10,
		height: 20,
		paddingLeft: 10,
		paddingRight: 10,
		fontWeight: 600,
	},

	deletedLabel: (theme) => ({
		...(theme.typography.caption as CSSObject),
		color: theme.palette.text.secondary,
	}),
} satisfies Record<string, Interpolation<Theme>>;
