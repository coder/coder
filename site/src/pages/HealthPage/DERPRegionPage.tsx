import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import type {
	DERPNodeReport,
	DERPRegionReport,
	HealthcheckReport,
	HealthSeverity,
} from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ChevronLeftIcon, CodeIcon, HashIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useOutletContext, useParams } from "react-router";
import { getLatencyColor } from "utils/latency";
import { pageTitle } from "utils/page";
import {
	BooleanPill,
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Logs,
	Main,
	Pill,
} from "./Content";

const DERPRegionPage: FC = () => {
	const theme = useTheme();
	const healthStatus = useOutletContext<HealthcheckReport>();
	const params = useParams() as { regionId: string };
	const regionId = Number(params.regionId);
	const {
		region,
		node_reports: reports,
		warnings,
		severity,
	} = healthStatus.derp.regions[regionId] as DERPRegionReport;

	return (
		<>
			<title>{pageTitle(region!.RegionName, "Health")}</title>

			<Header>
				<hgroup>
					<Link
						css={{
							fontSize: 12,
							textDecoration: "none",
							color: theme.palette.text.secondary,
							fontWeight: 500,
							display: "inline-flex",
							alignItems: "center",
							"&:hover": {
								color: theme.palette.text.primary,
							},
							marginBottom: 8,
							lineHeight: "1.2",
						}}
						to="/health/derp"
					>
						<ChevronLeftIcon
							className="size-icon-xs"
							css={{ verticalAlign: "middle", marginRight: 8 }}
						/>
						Back to DERP
					</Link>
					<HeaderTitle>
						<HealthyDot severity={severity as HealthSeverity} />
						{region!.RegionName}
					</HeaderTitle>
				</hgroup>
			</Header>

			<Main>
				{warnings.map((warning) => {
					return (
						<Alert
							actions={<HealthMessageDocsLink {...warning} />}
							key={warning.code}
							severity="warning"
						>
							{warning.message}
						</Alert>
					);
				})}

				<section>
					<div css={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
						<Tooltip delayDuration={0}>
							<TooltipTrigger asChild>
								<Pill icon={<HashIcon className="size-icon-sm" />}>
									{region!.RegionID}
								</Pill>
							</TooltipTrigger>
							<TooltipContent side="bottom">Region ID</TooltipContent>
						</Tooltip>
						<Tooltip delayDuration={0}>
							<TooltipTrigger asChild>
								<Pill icon={<CodeIcon className="size-icon-sm" />}>
									{region!.RegionCode}
								</Pill>
							</TooltipTrigger>
							<TooltipContent side="bottom">Region Code</TooltipContent>
						</Tooltip>
						<BooleanPill value={region!.EmbeddedRelay}>
							Embedded Relay
						</BooleanPill>
					</div>
				</section>

				{reports.map((report) => {
					report = report as DERPNodeReport; // Can technically be null
					const { node, client_logs: logs } = report;
					const latencyColor = getLatencyColor(
						theme,
						report.round_trip_ping_ms,
					);
					return (
						<section
							key={node!.HostName}
							css={{
								border: `1px solid ${theme.palette.divider}`,
								borderRadius: 8,
								fontSize: 14,
							}}
						>
							<header css={reportStyles.header}>
								<div>
									<h4 css={reportStyles.title}>{node!.HostName}</h4>
									<div css={reportStyles.ports}>
										<span>DERP Port: {node!.DERPPort ?? "None"}</span>
										<span>STUN Port: {node!.STUNPort ?? "None"}</span>
									</div>
								</div>

								<div css={reportStyles.pills}>
									<Tooltip delayDuration={0}>
										<TooltipTrigger asChild>
											<Pill
												css={{ color: latencyColor }}
												icon={<StatusCircle color={latencyColor} />}
											>
												{report.round_trip_ping_ms}ms
											</Pill>
										</TooltipTrigger>
										<TooltipContent side="bottom">
											Round trip ping
										</TooltipContent>
									</Tooltip>
									<BooleanPill value={report.can_exchange_messages}>
										Exchange Messages
									</BooleanPill>
									<BooleanPill value={report.uses_websocket}>
										Websocket
									</BooleanPill>
								</div>
							</header>
							<Logs lines={logs?.flat() ?? []} css={reportStyles.logs} />
							{report.client_errs.length > 0 && (
								<Logs
									lines={report.client_errs.flat()}
									css={[reportStyles.logs, reportStyles.clientErrors]}
								/>
							)}
						</section>
					);
				})}
			</Main>
		</>
	);
};

type StatusCircleProps = { color: string };

const StatusCircle: FC<StatusCircleProps> = ({ color }) => {
	return (
		<div
			css={{
				display: "flex",
				alignItems: "center",
				justifyContent: "center",
			}}
		>
			<div
				css={{
					width: 8,
					height: 8,
					backgroundColor: color,
					borderRadius: 9999,
				}}
			/>
		</div>
	);
};

const reportStyles = {
	header: {
		padding: 24,
		display: "flex",
		justifyContent: "space-between",
		alignItems: "center",
	},
	title: {
		fontWeight: 500,
		margin: 0,
		lineHeight: "1",
	},
	pills: {
		display: "flex",
		gap: 8,
		alignItems: "center",
	},
	ports: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 8,
		color: theme.palette.text.secondary,
		fontSize: 12,
		lineHeight: "1.2",
		marginTop: 8,
	}),
	divider: (theme) => ({
		height: 1,
		backgroundColor: theme.palette.divider,
	}),
	logs: (theme) => ({
		borderBottomLeftRadius: 8,
		borderBottomRightRadius: 8,
		borderTop: `1px solid ${theme.palette.divider}`,
	}),
	clientErrors: (theme) => ({
		background: theme.roles.error.background,
		color: theme.roles.error.text,
	}),
} satisfies Record<string, Interpolation<Theme>>;

export default DERPRegionPage;
