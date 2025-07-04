import type { Interpolation, Theme } from "@emotion/react";
import Button from "@mui/material/Button";
import Collapse from "@mui/material/Collapse";
import Divider from "@mui/material/Divider";
import Skeleton from "@mui/material/Skeleton";
import { xrayScan } from "api/queries/integrations";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentMetadata,
} from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
import { useProxy } from "contexts/ProxyContext";
import {
	type FC,
	useCallback,
	useEffect,
	useLayoutEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useQuery } from "react-query";
import AutoSizer from "react-virtualized-auto-sizer";
import type { FixedSizeList as List, ListOnScrollProps } from "react-window";
import { AgentLatency } from "./AgentLatency";
import { AGENT_LOG_LINE_HEIGHT } from "./AgentLogs/AgentLogLine";
import { AgentLogs } from "./AgentLogs/AgentLogs";
import { useAgentLogs } from "./AgentLogs/useAgentLogs";
import { AgentMetadata } from "./AgentMetadata";
import { AgentStatus } from "./AgentStatus";
import { AgentVersion } from "./AgentVersion";
import { AppLink } from "./AppLink/AppLink";
import { DownloadAgentLogsButton } from "./DownloadAgentLogsButton";
import { PortForwardButton } from "./PortForwardButton";
import { SSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton/VSCodeDesktopButton";
import { XRayScanAlert } from "./XRayScanAlert";

export interface AgentRowProps {
	agent: WorkspaceAgent;
	workspace: Workspace;
	showApps: boolean;
	showBuiltinApps?: boolean;
	sshPrefix?: string;
	hideSSHButton?: boolean;
	hideVSCodeDesktopButton?: boolean;
	serverVersion: string;
	serverAPIVersion: string;
	onUpdateAgent: () => void;
	template: Template;
	storybookAgentMetadata?: WorkspaceAgentMetadata[];
}

export const AgentRow: FC<AgentRowProps> = ({
	agent,
	workspace,
	template,
	showApps,
	showBuiltinApps = true,
	hideSSHButton,
	hideVSCodeDesktopButton,
	serverVersion,
	serverAPIVersion,
	onUpdateAgent,
	storybookAgentMetadata,
	sshPrefix,
}) => {
	// XRay integration
	const xrayScanQuery = useQuery(
		xrayScan({ workspaceId: workspace.id, agentId: agent.id }),
	);

	// Apps visibility
	const visibleApps = agent.apps.filter((app) => !app.hidden);
	const hasAppsToDisplay = !hideVSCodeDesktopButton || visibleApps.length > 0;
	const shouldDisplayApps =
		showApps &&
		((agent.status === "connected" && hasAppsToDisplay) ||
			agent.status === "connecting");
	const hasVSCodeApp =
		agent.display_apps.includes("vscode") ||
		agent.display_apps.includes("vscode_insiders");
	const showVSCode = hasVSCodeApp && !hideVSCodeDesktopButton;

	const hasStartupFeatures = Boolean(agent.logs_length);
	const { proxy } = useProxy();
	const [showLogs, setShowLogs] = useState(
		["starting", "start_timeout"].includes(agent.lifecycle_state) &&
			hasStartupFeatures,
	);
	const agentLogs = useAgentLogs({
		workspaceId: workspace.id,
		agentId: agent.id,
		agentLifeCycleState: agent.lifecycle_state,
		enabled: showLogs,
	});
	const logListRef = useRef<List>(null);
	const logListDivRef = useRef<HTMLDivElement>(null);
	const startupLogs = useMemo(() => {
		const allLogs = agentLogs || [];

		const logs = [...allLogs];
		if (agent.logs_overflowed) {
			logs.push({
				id: -1,
				level: "error",
				output: "Startup logs exceeded the max size of 1MB!",
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

	return (
		<Stack
			key={agent.id}
			direction="column"
			spacing={0}
			css={[
				styles.agentRow,
				styles[`agentRow-${agent.status}`],
				styles[`agentRow-lifecycle-${agent.lifecycle_state}`],
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
							<AgentVersion
								agent={agent}
								serverVersion={serverVersion}
								serverAPIVersion={serverAPIVersion}
								onUpdate={onUpdateAgent}
							/>
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

				{showBuiltinApps && (
					<div css={{ display: "flex" }}>
						{/*{!hideSSHButton && agent.display_apps.includes("ssh_helper") && (
							<SSHButton
								workspaceName={workspace.name}
								agentName={agent.name}
								sshPrefix={sshPrefix}
							/>
						)}*/}
						{proxy.preferredWildcardHostname &&
							proxy.preferredWildcardHostname !== "" &&
							agent.display_apps.includes("port_forwarding_helper") && (
								<PortForwardButton
									host={proxy.preferredWildcardHostname}
									workspaceName={workspace.name}
									agent={agent}
									username={workspace.owner_name}
									workspaceID={workspace.id}
									template={template}
								/>
							)}
					</div>
				)}
			</header>

			{xrayScanQuery.data && <XRayScanAlert scan={xrayScanQuery.data} />}

			<div css={styles.content}>
				{agent.status === "connected" && (
					<section css={styles.apps}>
						{shouldDisplayApps && (
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
								{visibleApps.map((app) => (
									<AppLink
										key={app.slug}
										app={app}
										agent={agent}
										workspace={workspace}
									/>
								))}
							</>
						)}

						{showBuiltinApps && agent.display_apps.includes("web_terminal") && (
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

				<AgentMetadata
					storybookMetadata={storybookAgentMetadata}
					agent={agent}
				/>
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
							variant="text"
							size="small"
							startIcon={<DropdownArrow close={showLogs} margin={false} />}
							onClick={() => setShowLogs((v) => !v)}
						>
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
} satisfies Record<string, Interpolation<Theme>>;
