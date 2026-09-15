import dayjs from "dayjs";
import {
	AppWindowIcon,
	CircleAlertIcon,
	CloudDownloadIcon,
	CloudUploadIcon,
	GaugeIcon,
	GitCompareArrowsIcon,
	RocketIcon,
	RotateCwIcon,
	SquareTerminalIcon,
	TerminalIcon,
	WrenchIcon,
} from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	type FC,
	type PropsWithChildren,
	useEffect,
	useMemo,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import type {
	DeploymentStats,
	HealthcheckReport,
	WorkspaceStatus,
} from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { HelpPopoverTitle } from "#/components/HelpPopover/HelpPopover";
import { Link } from "#/components/Link/Link";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { getDisplayWorkspaceStatus } from "#/utils/workspace";

interface DeploymentBannerViewProps {
	health?: HealthcheckReport;
	stats?: DeploymentStats;
	fetchStats?: () => void;
}

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

			<div className="ml-auto flex mr-3 items-center gap-8 text-content-primary">
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

const ActiveConnections: FC<{
	sessionCount?: DeploymentStats["session_count"];
}> = ({ sessionCount }) => {
	const applications = Object.entries(sessionCount?.session_counts ?? {})
		.filter(([, count]) => count > 0)
		.map(([id, count]) => ({ id, count, app: sessionCount?.apps[id] }))
		.sort(
			(first, second) =>
				second.count - first.count ||
				(first.app?.display_name || first.id).localeCompare(
					second.app?.display_name || second.id,
					"en-US",
				) ||
				first.id.localeCompare(second.id, "en-US"),
		);

	return (
		<div className="flex items-center">
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button
							variant="subtle"
							size="xs"
							className="mr-4 p-0 font-mono text-xs text-content-primary"
						>
							Active Connections
						</Button>
					</TooltipTrigger>
					<TooltipContent
						side="top"
						hideWhenDetached
						className="w-72 p-4 font-sans font-normal shadow-md"
					>
						<HelpPopoverTitle>Connections by family</HelpPopoverTitle>
						<dl className="m-0 grid gap-3 py-2 text-sm">
							{[
								{
									name: "VS Code",
									count: sessionCount?.vscode,
									icon: (
										<ExternalImage
											src="/icon/code.svg"
											alt=""
											className="size-4"
										/>
									),
								},
								{
									name: "JetBrains",
									count: sessionCount?.jetbrains,
									icon: (
										<ExternalImage
											src="/icon/jetbrains.svg"
											alt=""
											className="size-4"
										/>
									),
								},
								{
									name: "SSH",
									count: sessionCount?.ssh,
									icon: <TerminalIcon className="size-4" />,
								},
								{
									name: "Web Terminal",
									count: sessionCount?.reconnecting_pty,
									icon: <SquareTerminalIcon className="size-4" />,
								},
							].map(({ name, count, icon }) => (
								<div
									key={name}
									className="flex items-center justify-between gap-4"
								>
									<dt className="flex items-center gap-3">
										{icon}
										{name}
									</dt>
									<dd className="m-0 font-mono font-medium tabular-nums text-content-primary">
										{count ?? "-"}
									</dd>
								</div>
							))}
						</dl>
						<p className="mb-0 mt-3 border-0 border-t border-solid border-border pt-3 text-xs leading-relaxed text-content-secondary">
							Includes all apps in each family. Unrecognized apps are not
							included.
						</p>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>
			<div className="flex gap-2 text-content-secondary">
				{sessionCount ? (
					applications.length > 0 ? (
						applications
							.slice(0, 4)
							.map(({ id, count, app }, index) => (
								<ActiveConnection
									key={`${id}:${app?.icon ?? ""}`}
									app={app}
									count={count}
									id={id}
									showSeparator={index > 0}
								/>
							))
					) : (
						<div>No active connections</div>
					)
				) : (
					<div>-</div>
				)}
				{applications.length > 4 && (
					<Popover>
						<PopoverTrigger asChild>
							<Button
								variant="subtle"
								size="xs"
								className="p-0 font-mono text-xs"
							>
								+{applications.length - 4} more
							</Button>
						</PopoverTrigger>
						<PopoverContent
							side="top"
							aria-label="More active connections"
							hideWhenDetached
							className="p-3 text-xs"
						>
							<ul className="m-0 grid list-none gap-3 p-0">
								{applications.slice(4).map(({ id, count, app }) => (
									<li
										key={`${id}:${app?.icon ?? ""}`}
										className="flex items-center gap-2"
									>
										<AppIcon icon={app?.icon} name={app?.display_name || id} />
										<span className="min-w-0 flex-1 break-words">
											{app?.display_name || id}
										</span>
										<span>{count}</span>
									</li>
								))}
							</ul>
						</PopoverContent>
					</Popover>
				)}
			</div>
		</div>
	);
};

const ActiveConnection: FC<{
	app?: DeploymentStats["session_count"]["apps"][string];
	count: number;
	id: string;
	showSeparator: boolean;
}> = ({ app, count, id, showSeparator }) => {
	const displayName = app?.display_name || id;
	const accessibleName = `${displayName}: ${count} active connections`;
	const showName = !app?.icon?.startsWith("/icon/");

	return (
		<>
			{showSeparator && <ValueSeparator />}
			<TooltipProvider delayDuration={100}>
				<Tooltip>
					<TooltipTrigger asChild>
						<div
							aria-label={accessibleName}
							className="flex items-center gap-1"
							role="img"
							// biome-ignore lint/a11y/noNoninteractiveTabindex: role="img" supplies a keyboard tooltip trigger for icon-only connection counts.
							tabIndex={0}
						>
							<AppIcon icon={app?.icon} name={displayName} />
							{showName && (
								<span className="max-w-32 truncate">{displayName}</span>
							)}
							{count}
						</div>
					</TooltipTrigger>
					<TooltipContent>{displayName}</TooltipContent>
				</Tooltip>
			</TooltipProvider>
		</>
	);
};

const AppIcon: FC<{ icon?: string; name: string }> = ({ icon, name }) => {
	const [hasFailed, setHasFailed] = useState(false);

	if (!icon?.startsWith("/icon/") || hasFailed) {
		return <AppWindowIcon className="size-icon-xs shrink-0" />;
	}

	return (
		<ExternalImage
			src={icon}
			alt={`${name} icon`}
			className="size-icon-xs shrink-0"
			onError={() => setHasFailed(true)}
		/>
	);
};

interface WorkspaceBuildValueProps {
	status: WorkspaceStatus;
	count?: number;
}

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
