import type {
	DeploymentStats,
	HealthcheckReport,
	WorkspaceStatus,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { HelpTooltipTitle } from "components/HelpTooltip/HelpTooltip";
import { JetBrainsIcon } from "components/Icons/JetBrainsIcon";
import { RocketIcon } from "components/Icons/RocketIcon";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import { VSCodeIcon } from "components/Icons/VSCodeIcon";
import { Link } from "components/Link/Link";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import dayjs from "dayjs";
import {
	AppWindowIcon,
	ChevronLeftIcon,
	ChevronRightIcon,
	CircleAlertIcon,
	CloudDownloadIcon,
	CloudUploadIcon,
	GaugeIcon,
	GitCompareArrowsIcon,
	RotateCwIcon,
	WrenchIcon,
} from "lucide-react";
import prettyBytes from "pretty-bytes";
import {
	type FC,
	type PropsWithChildren,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import { getDisplayWorkspaceStatus } from "utils/workspace";

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

	// biome-ignore lint/correctness/useExhaustiveDependencies(timeUntilRefresh): periodic refresh
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

	// ── Slide navigation state ──────────────────────────────
	const wrapperRef = useRef<HTMLDivElement>(null);
	const trackRef = useRef<HTMLDivElement>(null);
	const [offset, setOffset] = useState(0);
	const [canSlide, setCanSlide] = useState({ left: false, right: false });

	const recalc = useCallback((nextOffset: number) => {
		const wrapper = wrapperRef.current;
		const track = trackRef.current;
		if (!wrapper || !track) return nextOffset;
		const max = Math.max(0, track.scrollWidth - wrapper.clientWidth);
		const clamped = Math.max(0, Math.min(max, nextOffset));
		setCanSlide({ left: clamped > 0, right: clamped < max - 1 });
		return clamped;
	}, []);

	// Snap to the nearest section boundary so we never land mid-label.
	const snap = useCallback((raw: number) => {
		const track = trackRef.current;
		if (!track) return raw;
		const sections = track.querySelectorAll<HTMLElement>(
			"[data-banner-section]",
		);
		let best = raw;
		let bestDist = Number.POSITIVE_INFINITY;
		for (const el of sections) {
			const boundary = el.offsetLeft;
			const dist = Math.abs(boundary - raw);
			if (dist < bestDist) {
				bestDist = dist;
				best = boundary;
			}
		}
		return best;
	}, []);

	const slideTo = useCallback(
		(px: number) => {
			const clamped = recalc(px);
			setOffset(clamped);
		},
		[recalc],
	);

	const slideNext = useCallback(() => {
		const step = Math.round((wrapperRef.current?.clientWidth ?? 400) * 0.9);
		slideTo(snap(offset + step));
	}, [offset, snap, slideTo]);

	const slidePrev = useCallback(() => {
		const step = Math.round((wrapperRef.current?.clientWidth ?? 400) * 0.9);
		slideTo(snap(offset - step));
	}, [offset, snap, slideTo]);

	// Recalculate on mount and resize.
	useEffect(() => {
		const onResize = () => slideTo(offset);
		onResize();
		window.addEventListener("resize", onResize);
		return () => window.removeEventListener("resize", onResize);
	}, [offset, slideTo]);

	// Recalculate whenever stats change (content may reflow).
	// biome-ignore lint/correctness/useExhaustiveDependencies(stats): content may reflow when stats change
	// biome-ignore lint/correctness/useExhaustiveDependencies(health): content may reflow when health changes
	useEffect(() => {
		slideTo(offset);
	}, [stats, health, offset, slideTo]);

	// When a keyboard user tabs to an element that's been translated
	// off-screen, auto-slide so the focused element is visible.
	// This satisfies WCAG 2.4.7 (Focus Visible).
	useEffect(() => {
		const wrapper = wrapperRef.current;
		if (!wrapper) return;

		const onFocusIn = (e: FocusEvent) => {
			const target = e.target as HTMLElement;
			const track = trackRef.current;
			if (!track || !wrapper.contains(target)) return;

			// Position of the focused element relative to the track.
			const elLeft = target.offsetLeft;
			const elRight = elLeft + target.offsetWidth;
			const visibleLeft = offset;
			const visibleRight = offset + wrapper.clientWidth;

			if (elLeft < visibleLeft) {
				// Element is clipped on the left — slide back.
				slideTo(snap(elLeft));
			} else if (elRight > visibleRight) {
				// Element is clipped on the right — slide forward.
				slideTo(snap(elRight - wrapper.clientWidth));
			}
		};

		wrapper.addEventListener("focusin", onFocusIn);
		return () => wrapper.removeEventListener("focusin", onFocusIn);
	}, [offset, snap, slideTo]);

	return (
		<div
			role="region"
			aria-label="Deployment statistics"
			className="sticky bottom-0 z-[1] flex h-9 w-full items-center
				whitespace-nowrap border-0 border-t border-solid border-border
				bg-surface-primary font-mono text-xs leading-none"
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
								className="flex h-full items-center justify-center px-3"
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
								<HelpTooltipTitle>
									We have detected problems with your Coder deployment.
								</HelpTooltipTitle>
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

			{/* ── Sliding track ─────────────────────── */}
			<div ref={wrapperRef} className="relative flex-1 overflow-hidden h-full">
				{/* Left edge fade — decorative */}
				<div
					aria-hidden="true"
					className={cn(
						"pointer-events-none absolute inset-y-0 left-0 z-10 w-12 transition-opacity duration-300",
						"bg-gradient-to-r from-surface-primary to-transparent",
						canSlide.left ? "opacity-100" : "opacity-0",
					)}
				/>
				{/* Right edge fade — decorative */}
				<div
					aria-hidden="true"
					className={cn(
						"pointer-events-none absolute inset-y-0 right-0 z-10 w-12 transition-opacity duration-300",
						"bg-gradient-to-l from-surface-primary to-transparent",
						canSlide.right ? "opacity-100" : "opacity-0",
					)}
				/>
				<div
					ref={trackRef}
					className="flex h-full items-center gap-8 pr-4 transition-transform duration-[400ms] ease-[cubic-bezier(.4,0,.2,1)]"
					style={{ transform: `translateX(-${offset}px)` }}
				>
					<div className="flex items-center" data-banner-section>
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

					<div className="flex items-center" data-banner-section>
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

					<div className="flex items-center" data-banner-section>
						<div className="mr-4 text-content-primary">Active Connections</div>

						<div className="flex gap-2 text-content-secondary">
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-1">
											<VSCodeIcon className="size-icon-xs [&_*]:fill-current" />
											{typeof stats?.session_count.vscode === "undefined"
												? "-"
												: stats?.session_count.vscode}
										</div>
									</TooltipTrigger>
									<TooltipContent>
										VS Code Editors with the Coder Remote Extension
									</TooltipContent>
								</Tooltip>
							</TooltipProvider>
							<ValueSeparator />
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-1">
											<JetBrainsIcon className="size-icon-xs [&_*]:fill-current" />
											{typeof stats?.session_count.jetbrains === "undefined"
												? "-"
												: stats?.session_count.jetbrains}
										</div>
									</TooltipTrigger>
									<TooltipContent>JetBrains Editors</TooltipContent>
								</Tooltip>
							</TooltipProvider>
							<ValueSeparator />
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-1">
											<TerminalIcon className="size-icon-xs" />
											{typeof stats?.session_count.ssh === "undefined"
												? "-"
												: stats?.session_count.ssh}
										</div>
									</TooltipTrigger>
									<TooltipContent>SSH Sessions</TooltipContent>
								</Tooltip>
							</TooltipProvider>
							<ValueSeparator />
							<TooltipProvider delayDuration={100}>
								<Tooltip>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-1">
											<AppWindowIcon className="size-icon-xs" />
											{typeof stats?.session_count.reconnecting_pty ===
											"undefined"
												? "-"
												: stats?.session_count.reconnecting_pty}
										</div>
									</TooltipTrigger>
									<TooltipContent>Web Terminal Sessions</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						</div>
					</div>

					<div
						className="ml-auto flex mr-3 items-center gap-8 text-content-primary"
						data-banner-section
					>
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
									The last time stats were aggregated. Workspaces report
									statistics periodically, so it may take a bit for these to
									update!
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
			</div>

			{/* ── Chevron navigation ────────────────── */}
			{(canSlide.left || canSlide.right) && (
				<nav
					aria-label="Scroll deployment banner"
					className="flex items-center gap-0.5 px-1.5 h-full shrink-0 border-0 border-l border-solid border-border"
				>
					{" "}
					<button
						type="button"
						aria-label="Scroll left"
						disabled={!canSlide.left}
						onClick={slidePrev}
						className={cn(
							"flex items-center justify-center size-6 rounded border-none bg-transparent cursor-pointer",
							"text-content-secondary transition-colors duration-150",
							"hover:bg-surface-tertiary hover:text-content-primary",
							"disabled:opacity-20 disabled:pointer-events-none",
						)}
					>
						<ChevronLeftIcon className="size-3.5" />
					</button>
					<button
						type="button"
						aria-label="Scroll right"
						disabled={!canSlide.right}
						onClick={slideNext}
						className={cn(
							"flex items-center justify-center size-6 rounded border-none bg-transparent cursor-pointer",
							"text-content-secondary transition-colors duration-150",
							"hover:bg-surface-tertiary hover:text-content-primary",
							"disabled:opacity-20 disabled:pointer-events-none",
						)}
					>
						<ChevronRightIcon className="size-3.5" />
					</button>
				</nav>
			)}
		</div>
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
