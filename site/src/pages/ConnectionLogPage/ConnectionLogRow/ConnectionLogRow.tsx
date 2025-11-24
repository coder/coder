import type { CSSObject, Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import type { ConnectionLog } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { Stack } from "components/Stack/Stack";
import { StatusPill } from "components/StatusPill/StatusPill";
import { TableCell } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
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
			<TableCell css={styles.connectionLogCell}>
				<Stack
					direction="row"
					alignItems="center"
					css={styles.connectionLogHeader}
					tabIndex={0}
				>
					<Stack
						direction="row"
						alignItems="center"
						css={styles.connectionLogHeaderInfo}
					>
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

						<Stack
							alignItems="center"
							css={styles.fullWidth}
							justifyContent="space-between"
							direction="row"
						>
							<Stack
								css={styles.connectionLogSummary}
								direction="row"
								alignItems="baseline"
								spacing={1}
							>
								<ConnectionLogDescription connectionLog={connectionLog} />
								<span css={styles.connectionLogTime}>
									{new Date(connectionLog.connect_time).toLocaleTimeString()}
									{connectionLog.ssh_info?.disconnect_time &&
										` → ${new Date(connectionLog.ssh_info.disconnect_time).toLocaleTimeString()}`}
								</span>
							</Stack>

							<Stack direction="row" alignItems="center">
								{code !== undefined && (
									<StatusPill
										code={code}
										isHttpCode={isWeb}
										label={isWeb ? "HTTP Status Code" : "SSH Exit Code"}
									/>
								)}
								<Tooltip
									title={
										<div css={styles.connectionLogInfoTooltip}>
											{connectionLog.ip && (
												<div>
													<h4 css={styles.connectionLogInfoheader}>IP:</h4>
													<div>{connectionLog.ip}</div>
												</div>
											)}
											{userAgent?.os.name && (
												<div>
													<h4 css={styles.connectionLogInfoheader}>OS:</h4>
													<div>{userAgent.os.name}</div>
												</div>
											)}
											{userAgent?.browser.name && (
												<div>
													<h4 css={styles.connectionLogInfoheader}>Browser:</h4>
													<div>
														{userAgent.browser.name} {userAgent.browser.version}
													</div>
												</div>
											)}
											{connectionLog.organization && (
												<div>
													<h4 css={styles.connectionLogInfoheader}>
														Organization:
													</h4>
													<Link
														component={RouterLink}
														to={`/organizations/${connectionLog.organization.name}`}
													>
														{connectionLog.organization.display_name ||
															connectionLog.organization.name}
													</Link>
												</div>
											)}
											{connectionLog.ssh_info?.disconnect_reason && (
												<div>
													<h4 css={styles.connectionLogInfoheader}>
														Close Reason:
													</h4>
													<div>{connectionLog.ssh_info?.disconnect_reason}</div>
												</div>
											)}
										</div>
									}
								>
									<InfoIcon
										css={(theme) => ({
											color: theme.palette.info.light,
										})}
									/>
								</Tooltip>
							</Stack>
						</Stack>
					</Stack>
				</Stack>
			</TableCell>
		</TimelineEntry>
	);
};

const styles = {
	connectionLogCell: {
		padding: "0 !important",
		border: 0,
	},

	connectionLogHeader: {
		padding: "16px 32px",
	},

	connectionLogHeaderInfo: {
		flex: 1,
	},

	connectionLogSummary: (theme) => ({
		...(theme.typography.body1 as CSSObject),
		fontFamily: "inherit",
	}),

	connectionLogTime: (theme) => ({
		color: theme.palette.text.secondary,
		fontSize: 12,
	}),

	connectionLogInfoheader: (theme) => ({
		margin: 0,
		color: theme.palette.text.primary,
		fontSize: 14,
		lineHeight: "150%",
		fontWeight: 600,
	}),

	connectionLogInfoTooltip: {
		display: "flex",
		flexDirection: "column",
		gap: 8,
	},

	fullWidth: {
		width: "100%",
	},
} satisfies Record<string, Interpolation<Theme>>;
