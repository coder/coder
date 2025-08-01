import type { Interpolation, Theme } from "@emotion/react";
import Collapse from "@mui/material/Collapse";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { useProxy } from "contexts/ProxyContext";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { AppStatuses } from "pages/WorkspacePage/AppStatuses";
import {
	type FC,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import AutoSizer from "react-virtualized-auto-sizer";
import type { FixedSizeList as List, ListOnScrollProps } from "react-window";
import { AgentApps, organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";
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
import { VSCodeDesktopButton } from "./VSCodeDesktopButton/VSCodeDesktopButton";
import { useAgentContainers } from "./useAgentContainers";
import { useAgentLogs } from "./useAgentLogs";

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
	const { browser_only } = useFeatureVisibility();
	const appSections = organizeAgentApps(agent.apps);
	const hasAppsToDisplay =
		!browser_only || appSections.some((it) => it.apps.length > 0);
	const shouldDisplayAgentApps =
		(agent.status === "connected" && hasAppsToDisplay) ||
		agent.status === "connecting";
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
	const agentLogs = useAgentLogs(agent, showLogs);
	const logListRef = useRef<List>(null);
	const logListDivRef = useRef<HTMLDivElement>(null);
	const startupLogs = useMemo(() => {
		const allLogs = agentLogs || [];

		const logs = [...allLogs];
		if (agent.logs_overflowed) {
			logs.push({
				id: -1,
				level: "error",
				output:
					"Startup logs exceeded the max size of 1MB, and will not continue to be written to the database! Logs will continue to be written to the /tmp/coder-startup-script.log file in the workspace.",
				created_at: new Date().toISOString(),
				source_id: "",
			});
		}
		return logs;
	}, [agentLogs, agent.logs_overflowed]);
	const [bottomOfLogs, setBottomOfLogs] = useState(true);

	useEffect(() => {
		setShowLogs(agent.lifecycle_state !== "ready" && hasStartupFeatures);
	}, [agent.lifecycle_state, hasStartupFeatures]);

	// This is a layout effect to remove flicker when we're scrolling to the bottom.
	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useLayoutEffect(() => {
		// If we're currently watching the bottom, we always want to stay at the bottom.
		if (bottomOfLogs && logListRef.current) {
			logListRef.current.scrollToItem(startupLogs.length - 1, "end");
		}
	}, [showLogs, startupLogs, bottomOfLogs]);

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

	let shouldDisplayAppsSection = shouldDisplayAgentApps;
	if (
		devcontainers &&
		devcontainers.find(
			// We only want to hide the parent apps by default when there are dev
			// containers that are either starting or running. If they are all in
			// the stopped state, it doesn't make sense to hide the parent apps.
			(dc) => dc.status === "running" || dc.status === "starting",
		) !== undefined &&
		!showParentApps
	) {
		shouldDisplayAppsSection = false;
	}

	// Check if any devcontainers have errors to gray out agent border
	const hasDevcontainerErrors = devcontainers?.some((dc) => dc.error);

	return (
		<Stack
			key={agent.id}
			direction="column"
			spacing={0}
			css={[
				styles.agentRow,
				styles[`agentRow-${agent.status}`],
				styles[`agentRow-lifecycle-${agent.lifecycle_state}`],
				hasDevcontainerErrors && styles.agentRowWithErrors,
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
					{devcontainers && devcontainers.length > 0 && (
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

				{agent.status === "connecting" && (
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
									logs={startupLogs.map((l) => ({
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

					<Stack css={{ padding: "12px 16px" }} direction="row" spacing={1}>
						<Button
							size="sm"
							variant="subtle"
							onClick={() => setShowLogs((v) => !v)}
						>
							<DropdownArrow close={showLogs} margin={false} />
							Logs
						</Button>
						<Divider orientation="vertical" variant="middle" flexItem />
						<DownloadAgentLogsButton workspaceId={workspace.id} agent={agent} />
					</Stack>
				</section>
			)}
		</Stack>
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
		maxHeight: 256,
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
