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

const statusBorderClassByStatus: Partial<
	Record<WorkspaceAgent["status"], string>
> = {
	connected: "border-border-success",
	connecting: "border-border-pending",
	timeout: "border-border-warning",
};

const statusBorderClassByLifecycle: Partial<
	Record<WorkspaceAgent["lifecycle_state"], string>
> = {
	starting: "border-border-pending",
	shutting_down: "border-border-primary",
	ready: "border-border-success",
	start_timeout: "border-border-warning",
	shutdown_timeout: "border-border-warning",
	start_error: "border-border-destructive",
	shutdown_error: "border-border-destructive",
	off: "border-border",
};

const getAgentBorderClass = (
	agent: WorkspaceAgent,
	hasErrorState: boolean,
): string => {
	if (hasErrorState) {
		return "border-border";
	}

	// Lifecycle state is more specific than connection status, so it wins.
	return (
		statusBorderClassByLifecycle[agent.lifecycle_state] ??
		statusBorderClassByStatus[agent.status] ??
		"border-border"
	);
};

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
	const borderClass = getAgentBorderClass(
		agent,
		Boolean(hasDevcontainerErrors || shouldShowWildcardWarning),
	);

	return (
		<div
			key={agent.id}
			className={cn(
				"flex max-w-full flex-col rounded-lg border border-solid bg-surface-primary text-sm shadow-md",
				borderClass,
			)}
		>
			<header className="flex flex-wrap items-center justify-between gap-4 px-4 pt-4 pl-8 leading-normal md:gap-6 [&:has(+_[role='alert'])]:pb-4">
				<div className="flex items-center gap-6 text-sm text-content-secondary">
					<div className="flex w-full items-center gap-4 md:w-auto">
						<AgentStatus agent={agent} />
						<span className="w-fit max-w-[260px] shrink-0 overflow-hidden text-ellipsis whitespace-nowrap text-base font-semibold text-content-primary md:overflow-visible">
							{agent.name}
						</span>
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

			<div className="flex flex-col gap-8 p-8">
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
					<section className="flex flex-wrap gap-4 [&:empty]:hidden">
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
					<section className="flex flex-wrap gap-4 [&:empty]:hidden">
						<Skeleton
							width={80}
							height={32}
							variant="rectangular"
							className="rounded"
						/>
						<Skeleton
							width={110}
							height={32}
							variant="rectangular"
							className="rounded"
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
				<section className="border-0 border-t border-solid border-border">
					<Collapse in={showLogs}>
						<AutoSizer disableHeight>
							{({ width }) => (
								<AgentLogs
									ref={logListRef}
									innerRef={logListDivRef}
									height={256}
									width={width}
									className="max-h-[420px] border-0 border-b border-solid border-border"
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
		</div>
	);
};
