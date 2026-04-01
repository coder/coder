import { ChevronLeftIcon, CodeIcon, HashIcon } from "lucide-react";
import type { FC } from "react";
import { Link, useOutletContext, useParams } from "react-router";
import type {
	DERPNodeReport,
	DERPRegionReport,
	HealthcheckReport,
	HealthSeverity,
} from "#/api/typesGenerated";
import { Alert } from "#/components/Alert/Alert";
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";
import { getLatencyColor } from "#/utils/latency";
import { pageTitle } from "#/utils/page";
import {
	BooleanPill,
	Header,
	HeaderTitle,
	HealthMessageDocsLink,
	HealthyDot,
	Logs,
	Main,
	Pill,
	StatusIcon,
} from "./Content";

interface NodeCheckRow {
	label: string;
	description: string;
	value: boolean | null;
}

const DERPRegionPage: FC = () => {
	const healthStatus = useOutletContext<HealthcheckReport>();
	const params = useParams() as { regionId: string };
	const regionId = Number(params.regionId);
	const {
		region,
		node_reports: reports,
		warnings,
		severity,
	} = healthStatus.derp.regions[regionId] as DERPRegionReport;

	if (!region) {
		return null;
	}

	return (
		<>
			<title>{pageTitle(region.RegionName, "Health")}</title>

			<Header>
				<hgroup>
					<Link
						className="text-xs no-underline text-content-secondary font-medium inline-flex items-center hover:text-content-primary mb-2 leading-tight"
						to="/health/derp"
					>
						<ChevronLeftIcon className="size-icon-xs align-middle mr-2" />
						Back to DERP
					</Link>
					<HeaderTitle>
						<HealthyDot severity={severity as HealthSeverity} />
						{region.RegionName}
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
							prominent
						>
							{warning.message}
						</Alert>
					);
				})}

				<section>
					<div className="flex flex-wrap gap-3">
						<Tooltip>
							<TooltipTrigger asChild>
								<Pill icon={<HashIcon className="size-icon-sm" />}>
									{region.RegionID}
								</Pill>
							</TooltipTrigger>
							<TooltipContent side="bottom">Region ID</TooltipContent>
						</Tooltip>
						<Tooltip>
							<TooltipTrigger asChild>
								<Pill icon={<CodeIcon className="size-icon-sm" />}>
									{region.RegionCode}
								</Pill>
							</TooltipTrigger>
							<TooltipContent side="bottom">Region Code</TooltipContent>
						</Tooltip>
						<Tooltip>
							<TooltipTrigger asChild>
								<BooleanPill value={region.EmbeddedRelay}>
									Embedded Relay
								</BooleanPill>
							</TooltipTrigger>
							<TooltipContent side="bottom">
								Whether this region uses a relay server embedded in the Coder
								deployment.
							</TooltipContent>
						</Tooltip>
					</div>
				</section>

				{reports.map((rawReport) => {
					if (!rawReport) {
						return null;
					}
					const report = rawReport as DERPNodeReport;
					const { node, client_logs: logs } = report;
					if (!node) {
						return null;
					}

					const latencyColor = getLatencyColor(report.round_trip_ping_ms);
					const latencyBackground = getLatencyColor(
						report.round_trip_ping_ms,
						"background",
					);
					const checks: NodeCheckRow[] = [
						{
							label: "Exchange Messages",
							description:
								"Whether DERP clients can relay messages through this node.",
							value: report.can_exchange_messages,
						},
						{
							label: "Direct HTTP Upgrade",
							description:
								"Whether the connection used a direct HTTP upgrade instead of falling back to WebSocket. Fallback may indicate the DERP upgrade header is being blocked.",
							value: !report.uses_websocket,
						},
						{
							label: "STUN Enabled",
							description: "Whether STUN is enabled on this node.",
							value: report.stun.Enabled,
						},
						{
							label: "STUN Reachable",
							description:
								"Whether this node responded to a STUN request successfully.",
							value: report.stun.CanSTUN,
						},
					];
					return (
						<section
							key={node.HostName}
							className="border border-solid border-border rounded-lg overflow-hidden text-sm"
						>
							<header className="p-6 flex justify-between items-center">
								<div>
									<h4 className="font-medium m-0 leading-none">
										{node.HostName}
									</h4>
									<div className="flex items-center gap-2 text-content-secondary text-xs leading-tight mt-2">
										<span>DERP Port: {node.DERPPort ?? "None"}</span>
										<span>STUN Port: {node.STUNPort ?? "None"}</span>
									</div>
								</div>

								<Tooltip>
									<TooltipTrigger asChild>
										<Pill
											className={latencyColor}
											icon={<StatusCircle background={latencyBackground} />}
										>
											{report.round_trip_ping_ms}ms
										</Pill>
									</TooltipTrigger>
									<TooltipContent side="bottom">Round trip ping</TooltipContent>
								</Tooltip>
							</header>

							<Table>
								<TableBody className="[&>tr>td:first-of-type]:border-l-0 [&>tr>td:last-child]:border-r-0 [&>tr:last-child>td]:border-b-0 [&>tr>td]:!rounded-none">
									{checks.map((check) => (
										<TableRow key={check.label}>
											<TableCell className="w-8">
												<StatusIcon value={check.value} />
											</TableCell>
											<TableCell className="font-medium whitespace-nowrap w-40">
												{check.label}
											</TableCell>
											<TableCell className="text-content-secondary">
												{check.description}
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
							<Logs
								lines={logs?.flat() ?? []}
								className="border-0 border-t border-solid border-border"
							/>
							{report.client_errs.length > 0 && (
								<Logs
									lines={report.client_errs.flat()}
									className="border-0 border-t border-solid border-border bg-surface-destructive text-content-destructive"
								/>
							)}
						</section>
					);
				})}
			</Main>
		</>
	);
};

type StatusCircleProps = { background: string };

const StatusCircle: FC<StatusCircleProps> = ({ background }) => {
	return (
		<div className="flex items-center justify-center">
			<div className={cn("size-2 rounded-full", background)} />
		</div>
	);
};

export default DERPRegionPage;
