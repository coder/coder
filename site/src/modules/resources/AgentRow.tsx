import type { Interpolation, Theme } from "@emotion/react";
import Collapse from "@mui/material/Collapse";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentMetadata,
	WorkspaceConnection,
	WorkspaceSession,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	connectionStatusColor,
	connectionStatusDot,
	connectionStatusLabel,
	connectionTypeLabel,
} from "./ConnectionStatus";
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from "components/Collapsible/Collapsible";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";

import { useProxy } from "contexts/ProxyContext";
import { SquareCheckBigIcon } from "lucide-react";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { AppStatuses } from "pages/WorkspacePage/AppStatuses";
import {
	type FC,
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { Link as RouterLink } from "react-router";
import AutoSizer from "react-virtualized-auto-sizer";
import type { FixedSizeList as List, ListOnScrollProps } from "react-window";
import { cn } from "utils/cn";
import { getLatencyColor } from "utils/latency";
import { formatDate, relativeTime } from "utils/time";
import { AgentApps, organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";
import { AgentExternal } from "./AgentExternal";
import { AgentLatency } from "./AgentLatency";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogs/AgentLogLine";
import { AgentLogs } from "./AgentLogs/AgentLogs";
import { AgentMetadata } from "./AgentMetadata";
import { AgentStatus } from "./AgentStatus";
import { AgentVersion } from "./AgentVersion";
import { DownloadAgentLogsButton } from "./DownloadAgentLogsButton";
import { PortForwardButton } from "./PortForwardButton";
import { AgentSSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { useAgentContainers } from "./useAgentContainers";
import { useAgentLogs } from "./useAgentLogs";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton/VSCodeDesktopButton";
import { WildcardHostnameWarning } from "./WildcardHostnameWarning";

interface AgentRowProps {
	agent: WorkspaceAgent;
	subAgents?: WorkspaceAgent[];
	workspace: Workspace;
	template: Template;
	initialMetadata?: WorkspaceAgentMetadata[];
	onUpdateAgent: () => void;
}

export const AgentRow: FC<AgentRowProps> = ({
	agent,
	subAgents,
	workspace,
	template,
	onUpdateAgent,
	initialMetadata,
}) => {
	const { browser_only, workspace_external_agent } = useFeatureVisibility();
	const appSections = organizeAgentApps(agent.apps);
	const hasAppsToDisplay =
		!browser_only || appSections.some((it) => it.apps.length > 0);
	const isExternalAgent = workspace.latest_build.has_external_agent;
	const shouldDisplayAgentApps =
		(agent.status === "connected" && hasAppsToDisplay) ||
		(agent.status === "connecting" && !isExternalAgent);
	const hasVSCodeApp =
		agent.display_apps.includes("vscode") ||
		agent.display_apps.includes("vscode_insiders");
	const showVSCode = hasVSCodeApp && !browser_only;

	const hasStartupFeatures = Boolean(agent.logs_length);
	const { proxy } = useProxy();
	const [showLogs, setShowLogs] = useState(
		["starting", "start_timeout"].includes(agent.lifecycle_state) &&
			hasStartupFeatures,
	);
	const agentLogs = useAgentLogs({ agentId: agent.id, enabled: showLogs });
	const logListRef = useRef<List>(null);
	const logListDivRef = useRef<HTMLDivElement>(null);
	const [bottomOfLogs, setBottomOfLogs] = useState(true);

	useEffect(() => {
		setShowLogs(agent.lifecycle_state !== "ready" && hasStartupFeatures);
	}, [agent.lifecycle_state, hasStartupFeatures]);

	// This is a layout effect to remove flicker when we're scrolling to the bottom.
	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useLayoutEffect(() => {
		// If we're currently watching the bottom, we always want to stay at the bottom.
		if (bottomOfLogs && logListRef.current) {
			logListRef.current.scrollToItem(agentLogs.length - 1, "end");
		}
	}, [showLogs, agentLogs, bottomOfLogs]);

	// This is a bit of a hack on the react-window API to get the scroll position.
	// If we're scrolled to the bottom, we want to keep the list scrolled to the bottom.
	// This makes it feel similar to a terminal that auto-scrolls downwards!
	const handleLogScroll = useCallback((props: ListOnScrollProps) => {
		if (
			props.scrollOffset === 0 ||
			props.scrollUpdateWasRequested ||
			!logListDivRef.current
		) {
			return;
		}
		// The parent holds the height of the list!
		const parent = logListDivRef.current.parentElement;
		if (!parent) {
			return;
		}
		const distanceFromBottom =
			logListDivRef.current.scrollHeight -
			(props.scrollOffset + parent.clientHeight);
		setBottomOfLogs(distanceFromBottom < AGENT_LOG_LINE_HEIGHT);
	}, []);

	const devcontainers = useAgentContainers(agent);

	// This is used to show the parent apps of the devcontainer.
	const [showParentApps, setShowParentApps] = useState(false);

	const anyRunningOrStartingDevcontainers =
		devcontainers?.find(
			(dc) => dc.status === "running" || dc.status === "starting",
		) !== undefined;

	// We only want to hide the parent apps by default when there are dev
	// containers that are either starting or running. If they are all in
	// the stopped state, it doesn't make sense to hide the parent apps.
	let shouldDisplayAppsSection = shouldDisplayAgentApps;
	if (anyRunningOrStartingDevcontainers && !showParentApps) {
		shouldDisplayAppsSection = false;
	}

	// Check if any devcontainers have errors to gray out agent border
	const hasDevcontainerErrors = devcontainers?.some((dc) => dc.error);

	const hasSubdomainApps = agent.apps?.some((app) => app.subdomain);
	const shouldShowWildcardWarning =
		hasSubdomainApps && !proxy.proxy?.wildcard_hostname;

	return (
		<div
			key={agent.id}
			className="flex flex-col max-w-full"
			css={[
				styles.agentRow,
				styles[`agentRow-${agent.status}`],
				styles[`agentRow-lifecycle-${agent.lifecycle_state}`],
				(hasDevcontainerErrors || shouldShowWildcardWarning) &&
					styles.agentRowWithErrors,
			]}
		>
			<header css={styles.header}>
				<div css={styles.agentInfo}>
					<div css={styles.agentNameAndStatus}>
						<AgentStatus agent={agent} />
						<span css={styles.agentName}>{agent.name}</span>
					</div>
					{agent.status === "connected" && (
						<>
							<AgentVersion agent={agent} onUpdate={onUpdateAgent} />
							<AgentLatency agent={agent} />
						</>
					)}
					{agent.status === "connecting" && (
						<>
							<Skeleton width={160} variant="text" />
							<Skeleton width={36} variant="text" />
						</>
					)}
				</div>

				<div className="flex items-center gap-2">
					{anyRunningOrStartingDevcontainers && (
						<Button
							variant="outline"
							size="sm"
							onClick={() => setShowParentApps((show) => !show)}
						>
							Show parent apps
							<DropdownArrow close={showParentApps} margin={false} />
						</Button>
					)}

					{!browser_only && agent.display_apps.includes("ssh_helper") && (
						<AgentSSHButton
							workspaceName={workspace.name}
							agentName={agent.name}
							workspaceOwnerUsername={workspace.owner_name}
						/>
					)}
					{proxy.preferredWildcardHostname !== "" &&
						agent.display_apps.includes("port_forwarding_helper") && (
							<PortForwardButton
								host={proxy.preferredWildcardHostname}
								workspace={workspace}
								agent={agent}
								template={template}
							/>
						)}
				</div>
			</header>

			<div css={styles.content}>
				{workspace.latest_app_status?.agent_id === agent.id && (
					<section>
						<h3 className="sr-only">App statuses</h3>
						<AppStatuses workspace={workspace} agent={agent} />
					</section>
				)}

				{workspace.task_id && (
					<Button asChild size="sm" variant="outline" className="w-fit">
						<RouterLink
							to={`/tasks/${workspace.owner_name}/${workspace.task_id}`}
						>
							<SquareCheckBigIcon />
							View task
						</RouterLink>
					</Button>
				)}

				{shouldShowWildcardWarning && <WildcardHostnameWarning />}

				{shouldDisplayAppsSection && (
					<section css={styles.apps}>
						{shouldDisplayAgentApps && (
							<>
								{showVSCode && (
									<VSCodeDesktopButton
										userName={workspace.owner_name}
										workspaceName={workspace.name}
										agentName={agent.name}
										folderPath={agent.expanded_directory}
										displayApps={agent.display_apps}
									/>
								)}
								{appSections.map((section, i) => (
									<AgentApps
										key={section.group ?? i}
										section={section}
										agent={agent}
										workspace={workspace}
									/>
								))}
							</>
						)}

						{agent.display_apps.includes("web_terminal") && (
							<TerminalLink
								workspaceName={workspace.name}
								agentName={agent.name}
								userName={workspace.owner_name}
							/>
						)}
					</section>
				)}

				{agent.status === "connecting" && !isExternalAgent && (
					<section css={styles.apps}>
						<Skeleton
							width={80}
							height={32}
							variant="rectangular"
							css={styles.buttonSkeleton}
						/>
						<Skeleton
							width={110}
							height={32}
							variant="rectangular"
							css={styles.buttonSkeleton}
						/>
					</section>
				)}

				{devcontainers && devcontainers.length > 0 && (
					<section className="flex flex-col gap-4">
						{devcontainers.map((devcontainer) => {
							return (
								<AgentDevcontainerCard
									key={devcontainer.id}
									devcontainer={devcontainer}
									workspace={workspace}
									template={template}
									wildcardHostname={proxy.preferredWildcardHostname}
									parentAgent={agent}
									subAgents={subAgents ?? []}
								/>
							);
						})}
					</section>
				)}

				{isExternalAgent &&
					(agent.status === "timeout" || agent.status === "connecting") &&
					workspace_external_agent && (
						<AgentExternal agent={agent} workspace={workspace} />
					)}

				<AgentMetadata initialMetadata={initialMetadata} agent={agent} />
			</div>

			{hasStartupFeatures && (
				<section
					css={(theme) => ({
						borderTop: `1px solid ${theme.palette.divider}`,
					})}
				>
					<Collapse in={showLogs}>
						<AutoSizer disableHeight>
							{({ width }) => (
								<AgentLogs
									ref={logListRef}
									innerRef={logListDivRef}
									height={256}
									width={width}
									css={styles.startupLogs}
									onScroll={handleLogScroll}
									overflowed={agent.logs_overflowed}
									logs={agentLogs.map((l) => ({
										id: l.id,
										level: l.level,
										output: l.output,
										sourceId: l.source_id,
										time: l.created_at,
									}))}
									sources={agent.log_sources}
								/>
							)}
						</AutoSizer>
					</Collapse>

					<div className="flex flex-row gap-2 px-4 py-3">
						<Button
							size="sm"
							variant="subtle"
							onClick={() => setShowLogs((v) => !v)}
						>
							<DropdownArrow close={showLogs} margin={false} />
							Logs
						</Button>
						<Divider orientation="vertical" variant="middle" flexItem />
						<DownloadAgentLogsButton agent={agent} />
					</div>
				</section>
			)}

			{agent.sessions && agent.sessions.length > 0 && (
				<AgentSessionsTable sessions={agent.sessions} />
			)}
		</div>
	);
};



// Build a single display label for a connection. Uses short_description
// (client identity like "Coder Desktop", "CLI") as primary, with the
// protocol/app type as secondary detail after a separator.
export function connectionLabel(conn: WorkspaceConnection): string {
	const typeLabel = conn.type
		? connectionTypeLabel(conn.type, conn.detail)
		: "";
	let desc = conn.short_description?.trim() ?? "";

	// The CLI currently reports "CLI ssh" for SSH connections. Trim the
	// protocol suffix so the rendered label does not duplicate SSH twice.
	if (conn.type === "ssh") {
		desc = desc.replace(/\s+ssh$/i, "");
	}

	if (desc && typeLabel) {
		const normalizedDesc = desc.toLowerCase();
		const normalizedType = typeLabel.toLowerCase();
		if (
			normalizedDesc === normalizedType ||
			normalizedDesc.endsWith(` ${normalizedType}`)
		) {
			return desc;
		}
		return `${desc} · ${typeLabel}`;
	}
	return desc || typeLabel || "Unknown";
}

// Produce a compact telemetry summary for a single connection,
// e.g. "0ms (Direct)", "Relay via Frankfurt 45ms", or null when
// telemetry is unavailable.
export function connectionTelemetrySummary(
	conn: WorkspaceConnection,
): string | null {
	if (
		conn.p2p === undefined &&
		conn.latency_ms === undefined &&
		!conn.home_derp
	) {
		return null;
	}

	if (conn.p2p === true) {
		if (conn.latency_ms !== undefined) {
			return `${Math.round(conn.latency_ms)}ms (Direct)`;
		}
		return "Direct";
	}

	const transport = conn.p2p === false ? "Relay" : "";
	const derp = conn.home_derp ? ` via ${conn.home_derp.name}` : "";
	const latency =
		conn.latency_ms !== undefined ? ` ${Math.round(conn.latency_ms)}ms` : "";
	return `${transport}${derp}${latency}`.trim() || null;
}

interface AgentSessionsTableProps {
	sessions: readonly WorkspaceSession[];
}

const AgentSessionsTable: FC<AgentSessionsTableProps> = ({ sessions }) => {
	return (
		<section
			css={(theme) => ({
				borderTop: `1px solid ${theme.palette.divider}`,
			})}
		>
			<div className="px-4 py-3">
				<h4 className="text-xs font-medium text-content-secondary mb-2">
					Sessions
				</h4>
				{sessions.map((session, idx) => (
					<SessionRow key={`${session.ip}-${idx}`} session={session} />
				))}
			</div>
		</section>
	);
};

function TelemetryBadge({ conn }: { conn: WorkspaceConnection }) {
	const summary = connectionTelemetrySummary(conn);
	if (!summary) {
		return <span className="text-xs text-content-secondary">—</span>;
	}

	return (
		<span
			className={cn(
				"text-xs rounded px-1.5 py-0.5",
				getLatencyColor(conn.latency_ms),
			)}
		>
			{summary}
		</span>
	);
}

const SessionRow: FC<{ session: WorkspaceSession }> = ({ session }) => {
	const [expanded, setExpanded] = useState(false);
	const hasMultiple = session.connections.length > 1;

	const displayName = session.client_hostname || session.ip || "Unknown";

	const activeCount = session.connections.length;

	return (
		<Collapsible open={expanded} onOpenChange={setExpanded}>
			<CollapsibleTrigger asChild disabled={!hasMultiple}>
				<div
					className={`flex items-center gap-3 py-2 rounded ${hasMultiple ? "cursor-pointer hover:bg-surface-secondary" : ""}`}
				>
					{hasMultiple && <DropdownArrow close={expanded} margin={false} />}
					{!hasMultiple && <div className="w-4" />}
					<span className="font-mono text-xs">{displayName}</span>
					<span className="text-xs text-content-secondary">
						{activeCount === 1
							? connectionLabel(session.connections[0])
							: `${activeCount} active connections`}
					</span>
					{activeCount === 1 && (
						<TelemetryBadge conn={session.connections[0]} />
					)}
					<span className="inline-flex items-center gap-1.5 text-xs">
						<span
							className={`inline-block h-2 w-2 rounded-full ${connectionStatusDot(session.status)}`}
						/>
						<span className={connectionStatusColor(session.status)}>
							{connectionStatusLabel(session.status)}
						</span>
					</span>
				</div>
			</CollapsibleTrigger>
			{hasMultiple && (
				<CollapsibleContent>
					<div className="pl-6 border-l border-border ml-2 mb-2">
						{session.connections.map((conn, idx) => (
							<div
								key={`${conn.type}-${conn.created_at}-${idx}`}
								className="flex items-center gap-3 py-1.5 text-xs"
							>
								<span>{connectionLabel(conn)}</span>
								<TelemetryBadge conn={conn} />
								<Tooltip>
									<TooltipTrigger asChild>
										<span className="text-content-secondary cursor-default">
											{relativeTime(conn.connected_at ?? conn.created_at)}
										</span>
									</TooltipTrigger>
									<TooltipContent>
										Connected at{" "}
										{formatDate(new Date(conn.connected_at ?? conn.created_at))}
									</TooltipContent>
								</Tooltip>
							</div>
						))}
					</div>
				</CollapsibleContent>
			)}
		</Collapsible>
	);
};

const styles = {
	agentRow: (theme) => ({
		fontSize: 14,
		border: `1px solid ${theme.palette.text.secondary}`,
		backgroundColor: theme.palette.background.default,
		borderRadius: 8,
		boxShadow: theme.shadows[3],
	}),

	"agentRow-connected": (theme) => ({
		borderColor: theme.palette.success.light,
	}),

	"agentRow-disconnected": (theme) => ({
		borderColor: theme.palette.divider,
	}),

	"agentRow-connecting": (theme) => ({
		borderColor: theme.palette.info.light,
	}),

	"agentRow-timeout": (theme) => ({
		borderColor: theme.palette.warning.light,
	}),

	"agentRow-lifecycle-created": {},

	"agentRow-lifecycle-starting": (theme) => ({
		borderColor: theme.palette.info.light,
	}),

	"agentRow-lifecycle-ready": (theme) => ({
		borderColor: theme.palette.success.light,
	}),

	"agentRow-lifecycle-start_timeout": (theme) => ({
		borderColor: theme.palette.warning.light,
	}),

	"agentRow-lifecycle-start_error": (theme) => ({
		borderColor: theme.palette.error.light,
	}),

	"agentRow-lifecycle-shutting_down": (theme) => ({
		borderColor: theme.palette.info.light,
	}),

	"agentRow-lifecycle-shutdown_timeout": (theme) => ({
		borderColor: theme.palette.warning.light,
	}),

	"agentRow-lifecycle-shutdown_error": (theme) => ({
		borderColor: theme.palette.error.light,
	}),

	"agentRow-lifecycle-off": (theme) => ({
		borderColor: theme.palette.divider,
	}),

	header: (theme) => ({
		padding: "16px 16px 0 32px",
		display: "flex",
		gap: 24,
		alignItems: "center",
		justifyContent: "space-between",
		flexWrap: "wrap",
		lineHeight: "1.5",

		"&:has(+ [role='alert'])": {
			paddingBottom: 16,
		},

		[theme.breakpoints.down("md")]: {
			gap: 16,
		},
	}),

	agentInfo: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 24,
		color: theme.palette.text.secondary,
		fontSize: 14,
	}),

	agentNameAndInfo: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 24,
		flexWrap: "wrap",

		[theme.breakpoints.down("md")]: {
			gap: 12,
		},
	}),

	content: {
		padding: 32,
		display: "flex",
		flexDirection: "column",
		gap: 32,
	},

	apps: (theme) => ({
		display: "flex",
		gap: 16,
		flexWrap: "wrap",

		"&:empty": {
			display: "none",
		},

		[theme.breakpoints.down("md")]: {
			marginLeft: 0,
			justifyContent: "flex-start",
		},
	}),

	agentDescription: (theme) => ({
		fontSize: 14,
		color: theme.palette.text.secondary,
	}),

	agentNameAndStatus: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 16,

		[theme.breakpoints.down("md")]: {
			width: "100%",
		},
	}),

	agentName: (theme) => ({
		whiteSpace: "nowrap",
		overflow: "hidden",
		textOverflow: "ellipsis",
		maxWidth: 260,
		fontWeight: 600,
		flexShrink: 0,
		width: "fit-content",
		fontSize: 16,
		color: theme.palette.text.primary,

		[theme.breakpoints.down("md")]: {
			overflow: "unset",
		},
	}),

	agentDataGroup: {
		display: "flex",
		alignItems: "baseline",
		gap: 48,
	},

	agentData: (theme) => ({
		display: "flex",
		flexDirection: "column",
		fontSize: 12,

		"& > *:first-of-type": {
			fontWeight: 500,
			color: theme.palette.text.secondary,
		},
	}),

	buttonSkeleton: {
		borderRadius: 4,
	},

	agentErrorMessage: (theme) => ({
		fontSize: 12,
		fontWeight: 400,
		marginTop: 4,
		color: theme.palette.warning.light,
	}),

	agentOS: {
		textTransform: "capitalize",
	},

	startupLogs: (theme) => ({
		maxHeight: 420,
		borderBottom: `1px solid ${theme.palette.divider}`,
		backgroundColor: theme.palette.background.paper,
		paddingTop: 16,

		// We need this to be able to apply the padding top from startupLogs
		"& > div": {
			position: "relative",
		},
	}),

	agentRowWithErrors: (theme) => ({
		borderColor: theme.palette.divider,
	}),
} satisfies Record<string, Interpolation<Theme>>;
