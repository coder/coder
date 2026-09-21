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
	WrenchIcon,
} from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	type FC,
	Fragment,
	memo,
	type PropsWithChildren,
	useEffect,
	useMemo,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import type {
	AppFamilyName,
	DeploymentStats,
	HealthcheckReport,
	SessionCountDeploymentStats,
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

const MAX_VISIBLE_APPS = 4;

const SESSION_FAMILIES: readonly {
	key: AppFamilyName;
	name: string;
	icon?: string;
}[] = [
	{ key: "vscode", name: "VS Code", icon: "/icon/code.svg" },
	{ key: "jetbrains", name: "JetBrains", icon: "/icon/jetbrains.svg" },
	{ key: "ssh", name: "SSH", icon: "/icon/terminal.svg" },
	{ key: "reconnecting_pty", name: "Web Terminal" },
];

const APP_COLLATOR = new Intl.Collator("en-US");

// Busiest first. The id tie-break keeps apps that share a display name, such
// as codium and vscodium, stably ordered across refreshes.
export const sortSessionApps = (
	apps: SessionCountDeploymentStats["apps"] = {},
) =>
	Object.entries(apps)
		.filter(([, app]) => app.count > 0)
		.map(([id, app]) => ({ ...app, id }))
		.sort(
			(first, second) =>
				second.count - first.count ||
				APP_COLLATOR.compare(first.display_name, second.display_name) ||
				APP_COLLATOR.compare(first.id, second.id),
		);

export const sumSessionFamilies = (
	apps: SessionCountDeploymentStats["apps"] = {},
) => {
	const totals = new Map<AppFamilyName, number>();
	for (const { family, count } of Object.values(apps)) {
		totals.set(family, (totals.get(family) ?? 0) + count);
	}
	return totals;
};

// The banner rerenders every second for its refresh countdown; these rows
// only change once per poll.
const ActiveConnections = memo(
	({ sessionCount }: { sessionCount?: SessionCountDeploymentStats }) => {
		const apps = sortSessionApps(sessionCount?.apps);
		const familyTotals = sumSessionFamilies(sessionCount?.apps);
		const visible = apps.slice(0, MAX_VISIBLE_APPS);
		const overflow = apps.slice(MAX_VISIBLE_APPS);

		return (
			<TooltipProvider delayDuration={TOOLTIP_DELAY_DURATION}>
				<div className="flex items-center">
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
								{SESSION_FAMILIES.map(({ key, name, icon }) => (
									<div
										key={key}
										className="flex items-center justify-between gap-4"
									>
										<dt className="flex items-center gap-3">
											<AppLabel icon={icon} name={name} showName />
										</dt>
										<dd className="m-0 font-mono font-medium tabular-nums text-content-primary">
											{sessionCount ? (familyTotals.get(key) ?? 0) : "-"}
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
					<div className="flex gap-2 text-content-secondary">
						{visible.length === 0 && (
							<div>{sessionCount ? "No active connections" : "-"}</div>
						)}
						{visible.map(({ id, count, display_name, icon }, index) => (
							<Fragment key={id}>
								{index > 0 && <ValueSeparator />}
								<ActiveConnection
									count={count}
									name={display_name}
									icon={icon}
								/>
							</Fragment>
						))}
						{overflow.length > 0 && (
							<Popover>
								<PopoverTrigger asChild>
									<Button
										variant="subtle"
										size="xs"
										className="p-0 font-mono text-xs"
									>
										+{overflow.length} more
									</Button>
								</PopoverTrigger>
								<PopoverContent
									side="top"
									aria-label="More active connections"
									hideWhenDetached
									className="p-3 text-xs"
								>
									<ul className="m-0 grid list-none gap-3 p-0">
										{overflow.map(({ id, display_name, count, icon }) => (
											<li key={id} className="flex items-center gap-2">
												<AppLabel
													icon={icon}
													name={display_name}
													showName
													nameClassName="min-w-0 flex-1 break-words"
												/>
												<span>{count}</span>
											</li>
										))}
									</ul>
								</PopoverContent>
							</Popover>
						)}
					</div>
				</div>
			</TooltipProvider>
		);
	},
);

const ActiveConnection: FC<{
	icon?: string;
	count: number;
	name: string;
}> = ({ icon, count, name }) => (
	<Tooltip>
		<TooltipTrigger asChild>
			<Button
				variant="subtle"
				size="xs"
				aria-label={`${name}: ${count} active connections`}
				className="gap-1 p-0 font-mono text-xs"
			>
				<AppLabel icon={icon} name={name} nameClassName="max-w-32 truncate" />
				{count}
			</Button>
		</TooltipTrigger>
		<TooltipContent>{name}</TooltipContent>
	</Tooltip>
);

const AppLabel: FC<{
	icon?: string;
	name: string;
	showName?: boolean;
	nameClassName?: string;
}> = ({ icon, name, showName, nameClassName }) => {
	// Keyed by path, not a boolean, so a later icon change retries.
	const [failedIcon, setFailedIcon] = useState<string>();
	// Only server-curated "/icon/" paths reach an image source.
	const src =
		icon?.startsWith("/icon/") && icon !== failedIcon ? icon : undefined;

	return (
		<>
			{src ? (
				<ExternalImage
					src={src}
					alt={showName ? "" : `${name} icon`}
					className="size-icon-xs shrink-0"
					onError={() => setFailedIcon(src)}
				/>
			) : (
				<AppWindowIcon className="size-icon-xs shrink-0" />
			)}
			{(showName || !src) && <span className={nameClassName}>{name}</span>}
		</>
	);
};

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
