import dayjs from "dayjs";
import {
	AppWindowIcon,
	BlocksIcon,
	CircleAlertIcon,
	CloudDownloadIcon,
	CloudUploadIcon,
	GaugeIcon,
	GitCompareArrowsIcon,
	RocketIcon,
	RotateCwIcon,
	SquareTerminalIcon,
	WrenchIcon,
} from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	type FC,
	Fragment,
	memo,
	type PropsWithChildren,
	type ReactNode,
	useEffect,
	useMemo,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import {
	type AppFamilyName,
	AppFamilyNames,
	type DeploymentStats,
	type HealthcheckReport,
	type SessionCountApp,
	type SessionCountDeploymentStats,
	type WorkspaceStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { HelpPopoverTitle } from "#/components/HelpPopover/HelpPopover";
import { Link } from "#/components/Link/Link";
import {
	TOOLTIP_DELAY_DURATION,
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getDisplayWorkspaceStatus } from "#/utils/workspace";

type DeploymentBannerViewProps = {
	health?: HealthcheckReport;
	stats?: DeploymentStats;
	fetchStats?: () => void;
};

export const DeploymentBannerView: FC<DeploymentBannerViewProps> = ({
	health,
	stats,
	fetchStats,
}) => {
	const aggregatedMinutes = useMemo(() => {
		if (!stats) {
			return;
		}
		return dayjs(stats.collected_at).diff(stats.aggregated_from, "minutes");
	}, [stats]);

	const [timeUntilRefresh, setTimeUntilRefresh] = useState(0);
	useEffect(() => {
		if (!stats || !fetchStats) {
			return;
		}

		let timeUntilRefresh = dayjs(stats.next_update_at).diff(
			stats.collected_at,
			"seconds",
		);
		setTimeUntilRefresh(timeUntilRefresh);
		let canceled = false;
		const loop = () => {
			if (canceled) {
				return undefined;
			}
			setTimeUntilRefresh(timeUntilRefresh--);
			if (timeUntilRefresh > 0) {
				return window.setTimeout(loop, 1000);
			}
			fetchStats();
		};
		const timeout = setTimeout(loop, 1000);
		return () => {
			canceled = true;
			clearTimeout(timeout);
		};
	}, [fetchStats, stats]);

	const lastAggregated = useMemo(() => {
		if (!stats) {
			return;
		}
		if (!fetchStats) {
			// Storybook!
			return "just now";
		}
		return dayjs().to(dayjs(stats.collected_at));
	}, [timeUntilRefresh, stats, fetchStats]);

	const healthErrors = health ? getHealthErrors(health) : [];
	const displayLatency = stats?.workspaces.connection_latency_ms.P50 || -1;

	return (
		<div
			className="sticky bottom-0 z-1 flex h-9 w-full items-center gap-8
		 		overflow-x-auto overflow-y-hidden whitespace-nowrap border-0 border-t border-solid border-border
				bg-surface-primary pr-4 font-mono text-xs leading-none"
		>
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						{healthErrors.length > 0 ? (
							<Link
								asChild
								className="flex p-3 bg-content-destructive"
								showExternalIcon={false}
							>
								<RouterLink
									to="/health"
									data-testid="deployment-health-trigger"
								>
									<CircleAlertIcon className="text-content-primary" />
								</RouterLink>
							</Link>
						) : (
							<div
								className="flex h-full items-center justify-center pl-3"
								data-testid="deployment-health-trigger"
							>
								<RocketIcon className="size-icon-sm" />
							</div>
						)}
					</TooltipTrigger>
					<TooltipContent
						className="ml-3 mb-1 p-4 text-sm text-content-primary
							border border-solid border-border pointer-events-none"
					>
						{healthErrors.length > 0 ? (
							<>
								<HelpPopoverTitle>
									We have detected problems with your Coder deployment.
								</HelpPopoverTitle>
								<div className="flex flex-col gap-1">
									{healthErrors.map((error) => (
										<HealthIssue key={error}>{error}</HealthIssue>
									))}
								</div>
							</>
						) : (
							"Status of your Coder deployment. Only visible for admins!"
						)}
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>

			<div className="flex items-center">
				<div className="mr-4 text-content-primary">Workspaces</div>
				<div className="flex gap-2 text-content-secondary">
					<WorkspaceBuildValue
						status="pending"
						count={stats?.workspaces.pending}
					/>
					<ValueSeparator />
					<WorkspaceBuildValue
						status="starting"
						count={stats?.workspaces.building}
					/>
					<ValueSeparator />
					<WorkspaceBuildValue
						status="running"
						count={stats?.workspaces.running}
					/>
					<ValueSeparator />
					<WorkspaceBuildValue
						status="stopped"
						count={stats?.workspaces.stopped}
					/>
					<ValueSeparator />
					<WorkspaceBuildValue
						status="failed"
						count={stats?.workspaces.failed}
					/>
				</div>
			</div>

			<div className="flex items-center">
				<TooltipProvider delayDuration={100}>
					<Tooltip>
						<TooltipTrigger asChild>
							<div className="mr-4 text-content-primary">Transmission</div>
						</TooltipTrigger>
						<TooltipContent>
							{`Activity in the last ~${aggregatedMinutes} minutes`}
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
				<div className="flex gap-2 text-content-secondary">
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="flex items-center gap-1">
									<CloudDownloadIcon className="size-icon-xs" />
									{stats ? prettyBytes(stats.workspaces.rx_bytes) : "-"}
								</div>
							</TooltipTrigger>
							<TooltipContent>Data sent to workspaces</TooltipContent>
						</Tooltip>
					</TooltipProvider>
					<ValueSeparator />
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="flex items-center gap-1">
									<CloudUploadIcon className="size-icon-xs" />
									{stats ? prettyBytes(stats.workspaces.tx_bytes) : "-"}
								</div>
							</TooltipTrigger>
							<TooltipContent>Data sent from workspaces</TooltipContent>
						</Tooltip>
					</TooltipProvider>
					<ValueSeparator />
					<TooltipProvider delayDuration={100}>
						<Tooltip>
							<TooltipTrigger asChild>
								<div className="flex items-center gap-1">
									<GaugeIcon className="size-icon-xs" />
									{displayLatency > 0
										? `${displayLatency?.toFixed(2)} ms`
										: "-"}
								</div>
							</TooltipTrigger>
							<TooltipContent>
								{displayLatency < 0
									? "No recent workspace connections have been made"
									: "The average latency of user connections to workspaces"}
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>
			</div>

			<ActiveConnections sessionCount={stats?.session_count} />

			<div className="ml-auto flex mr-3 items-center gap-8 pl-8 text-content-primary">
				<TooltipProvider delayDuration={100}>
					<Tooltip>
						<TooltipTrigger asChild>
							<div className="flex items-center gap-1">
								<GitCompareArrowsIcon className="size-icon-xs" />
								{lastAggregated}
							</div>
						</TooltipTrigger>
						<TooltipContent
							className="max-w-xs"
							collisionPadding={{ right: 20 }}
						>
							The last time stats were aggregated. Workspaces report statistics
							periodically, so it may take a bit for these to update!
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

				<TooltipProvider delayDuration={100}>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								className="font-mono [&_svg]:mr-1"
								onClick={() => {
									if (fetchStats) {
										fetchStats();
									}
								}}
								variant="subtle"
								size="icon"
							>
								<RotateCwIcon />
								{timeUntilRefresh}s
							</Button>
						</TooltipTrigger>
						<TooltipContent
							className="max-w-xs"
							collisionPadding={{ right: 20 }}
						>
							A countdown until stats are fetched again. Click to refresh!
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>
			</div>
		</div>
	);
};

/**
 * Banner slots in bar order. A family added in Go fails typecheck until it
 * gets a slot here, or null to total under Other.
 */
const SESSION_FAMILIES = {
	vscode: {
		name: "Visual Studio Code",
		icon: <ExternalImage src="/icon/code.svg" className="size-icon-xs" />,
	},
	jetbrains: {
		name: "JetBrains",
		icon: <ExternalImage src="/icon/jetbrains.svg" className="size-icon-xs" />,
	},
	ssh: { name: "SSH", icon: <SquareTerminalIcon className="size-icon-xs" /> },
	reconnecting_pty: {
		name: "Web Terminal",
		icon: <AppWindowIcon className="size-icon-xs" />,
	},
	unknown: { name: "Other", icon: <BlocksIcon className="size-icon-xs" /> },
	sftp: null,
} satisfies Record<AppFamilyName, { name: string; icon: ReactNode } | null>;

const FAMILY_SLOTS = Object.entries(SESSION_FAMILIES).flatMap(
	([name, slot]) => {
		const key = AppFamilyNames.find((family) => family === name);
		return key && slot ? [{ key, ...slot }] : [];
	},
);

const APP_COLLATOR = new Intl.Collator("en-US", { numeric: true });

/** Groups active apps by banner slot, sorted by name so rows stay put. */
export const groupSessionApps = (
	apps: SessionCountDeploymentStats["apps"] = {},
) => {
	const groups = new Map<AppFamilyName, (SessionCountApp & { id: string })[]>();
	const sorted = Object.entries(apps)
		.filter(([, app]) => app.count > 0)
		.map(([id, app]) => ({ ...app, id }))
		.sort(
			(first, second) =>
				APP_COLLATOR.compare(first.display_name, second.display_name) ||
				APP_COLLATOR.compare(first.id, second.id),
		);
	for (const app of sorted) {
		const family = SESSION_FAMILIES[app.family] ? app.family : "unknown";
		groups.set(family, [...(groups.get(family) ?? []), app]);
	}
	return groups;
};

/**
 * Memoized because the banner rerenders every second for its countdown, while
 * these rows change once per poll.
 */
const ActiveConnections = memo(
	({ sessionCount }: { sessionCount?: SessionCountDeploymentStats }) => {
		const groups = groupSessionApps(sessionCount?.apps);

		return (
			<TooltipProvider delayDuration={TOOLTIP_DELAY_DURATION}>
				<div className="flex items-center">
					<div className="mr-4 text-content-primary">Active Connections</div>
					<div className="flex gap-2 text-content-secondary">
						{FAMILY_SLOTS.map(({ key, name, icon }, index) => {
							const apps = groups.get(key) ?? [];
							const count = apps.reduce((sum, app) => sum + app.count, 0);
							return (
								<Fragment key={key}>
									{index > 0 && <ValueSeparator />}
									<Tooltip>
										<TooltipTrigger asChild>
											<Button
												variant="subtle"
												size="xs"
												aria-label={
													sessionCount
														? `${name}: ${count} active connections`
														: `${name}: - loading active connections`
												}
												className="min-w-0 gap-1 p-0 font-mono text-xs [&>img]:p-0 [&>svg]:p-0"
											>
												{icon}
												{sessionCount ? count : "-"}
											</Button>
										</TooltipTrigger>
										<TooltipContent className="p-3 font-sans text-xs">
											<div className="mb-2 font-medium text-content-primary">
												{name}
											</div>
											{!sessionCount ? (
												<div>Loading active connections</div>
											) : apps.length === 0 ? (
												<div>No active connections</div>
											) : (
												<ul className="m-0 grid max-h-64 list-none gap-2 overflow-y-auto p-0">
													{apps.map((app) => (
														<li
															key={app.id}
															className="flex items-center gap-2"
														>
															<AppLabel
																icon={app.icon}
																name={app.display_name}
															/>
															<span className="font-mono">{app.count}</span>
														</li>
													))}
												</ul>
											)}
										</TooltipContent>
									</Tooltip>
								</Fragment>
							);
						})}
					</div>
				</div>
			</TooltipProvider>
		);
	},
);

/** Renders an app's icon and name. Only bundled "/icon/" paths load. */
const AppLabel: FC<{ icon?: string; name: string }> = ({ icon, name }) => (
	<>
		{icon?.startsWith("/icon/") ? (
			<ExternalImage src={icon} className="size-icon-xs shrink-0" />
		) : (
			<AppWindowIcon className="size-icon-xs shrink-0" />
		)}
		<span className="min-w-0 flex-1 break-words">{name}</span>
	</>
);

type WorkspaceBuildValueProps = {
	status: WorkspaceStatus;
	count?: number;
};

const WorkspaceBuildValue: FC<WorkspaceBuildValueProps> = ({
	status,
	count,
}) => {
	const displayStatus = getDisplayWorkspaceStatus(status);
	let statusText = displayStatus.text;
	let icon = displayStatus.icon;
	if (status === "starting") {
		icon = <WrenchIcon className="size-icon-xs" />;
		statusText = "Building";
	}

	return (
		<TooltipProvider delayDuration={100}>
			<Tooltip>
				<TooltipTrigger asChild>
					<Link asChild showExternalIcon={false}>
						<RouterLink
							to={`/workspaces?filter=${encodeURIComponent(`status:${status}`)}`}
						>
							<div className="flex items-center gap-1 text-xs">
								{icon}
								{typeof count === "undefined" ? "-" : count}
							</div>
						</RouterLink>
					</Link>
				</TooltipTrigger>
				<TooltipContent>{`${statusText} Workspaces`}</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};

const ValueSeparator: FC = () => {
	return <div className="text-content-disabled self-center">/</div>;
};

const HealthIssue: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex items-center gap-1">
			<CircleAlertIcon className="size-icon-sm text-border-destructive" />
			{children}
		</div>
	);
};

const getHealthErrors = (health: HealthcheckReport) => {
	const warnings: string[] = [];
	const sections = [
		"access_url",
		"database",
		"derp",
		"websocket",
		"workspace_proxy",
	] as const;
	const messages: Record<(typeof sections)[number], string> = {
		access_url: "Your access URL may be configured incorrectly.",
		database: "Your database is unhealthy.",
		derp: "We're noticing DERP proxy issues.",
		websocket: "We're noticing websocket issues.",
		workspace_proxy: "We're noticing workspace proxy issues.",
	} as const;

	for (const section of sections) {
		if (health[section].severity === "error" && !health[section].dismissed) {
			warnings.push(messages[section]);
		}
	}

	return warnings;
};
