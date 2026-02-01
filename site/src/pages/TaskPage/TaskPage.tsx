import { API } from "api/api";
import { getErrorDetail, getErrorMessage, isApiError } from "api/errors";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";
import {
	startWorkspace,
	workspaceByOwnerAndName,
	workspacePermissions,
} from "api/queries/workspaces";
import type {
	Task,
	Workspace,
	WorkspaceAgent,
	WorkspaceStatus,
} from "api/typesGenerated";
import isChromatic from "chromatic/isChromatic";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Spinner } from "components/Spinner/Spinner";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import {
	AlertTriangleIcon,
	ArrowLeftIcon,
	Code2,
	CopyIcon,
	RotateCcwIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { AgentLogs } from "modules/resources/AgentLogs/AgentLogs";
import { useAgentLogs } from "modules/resources/useAgentLogs";
import { getAllAppsWithAgent } from "modules/tasks/apps";
import { NewTaskDialog } from "modules/tasks/NewTaskDialog/NewTaskDialog";
import { TasksSidebar } from "modules/tasks/TasksSidebar/TasksSidebar";
import { WorkspaceErrorDialog } from "modules/workspaces/ErrorDialog/WorkspaceErrorDialog";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import {
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { TaskApps } from "./TaskApps";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { Link as RouterLink, useParams } from "react-router";
import type { FixedSizeList } from "react-window";
import { cn } from "utils/cn";
import { pageTitle } from "utils/page";
import {
	getActiveTransitionStats,
	WorkspaceBuildProgress,
} from "../WorkspacePage/WorkspaceBuildProgress";
import { ModifyPromptDialog } from "./ModifyPromptDialog";
import { TaskAppIFrame } from "./TaskAppIframe";
import { TaskTopbar } from "./TaskTopbar";

const TaskPageLayout: FC<PropsWithChildren> = ({ children }) => {
	return (
		<div className="flex items-stretch h-full">
			<TasksSidebar />
			<div className="flex flex-col h-full flex-1">{children}</div>
		</div>
	);
};

const TaskPage = () => {
	const [isModifyDialogOpen, setIsModifyDialogOpen] = useState(false);
	const { taskId, username } = useParams() as {
		taskId: string;
		username: string;
	};
	const { data: task, ...taskQuery } = useQuery({
		queryKey: ["tasks", username, taskId],
		queryFn: () => API.getTask(username, taskId),
		refetchInterval: ({ state }) => {
			return state.error ? false : 5_000;
		},
	});

	// Fetch all tasks to determine position
	const { data: allTasks } = useQuery({
		queryKey: ["tasks", { owner: username }],
		queryFn: () => API.getTasks({ owner: username }),
		enabled: !!username,
	});

	// Sort tasks by workspace_id and find current task index
	const sortedTasks = allTasks
		? [...allTasks].sort((a, b) =>
				(a.workspace_id ?? "").localeCompare(b.workspace_id ?? ""),
			)
		: [];
	const taskIndex = sortedTasks.findIndex((t) => t.id === taskId);

	const { data: workspace, ...workspaceQuery } = useQuery({
		...workspaceByOwnerAndName(username, task?.workspace_name ?? ""),
		enabled: task !== undefined,
		refetchInterval: ({ state }) => {
			return state.error ? false : 5_000;
		},
	});
	const { data: permissions } = useQuery(workspacePermissions(workspace));
	const refetch = taskQuery.error ? taskQuery.refetch : workspaceQuery.refetch;
	const error = taskQuery.error ?? workspaceQuery.error;
	const waitingStatuses: WorkspaceStatus[] = ["starting", "pending"];

	if (error) {
		return (
			<TaskPageLayout>
				<title>{pageTitle("Error loading task")}</title>

				<div className="w-full min-h-80 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							{getErrorMessage(error, "Failed to load task")}
						</h3>
						<span className="text-content-secondary text-sm">
							{getErrorDetail(error)}
						</span>
						<div className="mt-4 flex items-center gap-2">
							<Button size="sm" variant="outline" asChild>
								<RouterLink to="/tasks">
									<ArrowLeftIcon />
									Back to tasks
								</RouterLink>
							</Button>
							<Button size="sm" onClick={() => refetch()}>
								<RotateCcwIcon />
								Try again
							</Button>
						</div>
					</div>
				</div>
			</TaskPageLayout>
		);
	}

	if (!task || !workspace) {
		return (
			<TaskPageLayout>
				<title>{pageTitle("Loading task")}</title>
				<Loader className="w-full h-full" />
			</TaskPageLayout>
		);
	}

	let content: ReactNode = null;
	const agent = selectAgent(workspace);

	if (waitingStatuses.includes(workspace.latest_build.status)) {
		content = (
			<BuildingWorkspace
				workspace={workspace}
				onEditPrompt={() => setIsModifyDialogOpen(true)}
			/>
		);
	} else if (workspace.latest_build.status === "failed") {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Task build failed
					</h3>
					<span className="text-content-secondary text-sm">
						Please check the logs for more details.
					</span>
					<Button size="sm" variant="outline" asChild className="mt-4">
						<RouterLink
							to={`/@${workspace.owner_name}/${workspace.name}/builds/${workspace.latest_build.build_number}`}
						>
							View logs
						</RouterLink>
					</Button>
				</div>
			</div>
		);
	} else if (workspace.latest_build.status !== "running") {
		content = (
			<WorkspaceNotRunning
				workspace={workspace}
				onEditPrompt={() => setIsModifyDialogOpen(true)}
			/>
		);
	} else if (agent && ["created", "starting"].includes(agent.lifecycle_state)) {
		content = <TaskStartingAgent agent={agent} />;
	} else {
		const allApps = getAllAppsWithAgent(workspace);

		// Third task (index 2) is headless - show special full-width agent view
		if (taskIndex === 2) {
			content = (
				<HeadlessAgentView
					task={task}
					workspace={workspace}
					canUpdatePermissions={permissions?.updateWorkspace ?? false}
				/>
			);
		} else {
			// First task (index 0) uses "code-server", second task (index 1) uses "mux"
			const leftPanelAppSlug = taskIndex === 1 ? "mux" : "code-server";
			const leftPanelApp = allApps.find((app) => app.slug === leftPanelAppSlug);
			content = (
				<PanelGroup autoSaveId="task" direction="horizontal">
					<Panel defaultSize={25} minSize={20}>
						{leftPanelApp ? (
							<TaskAppIFrame active workspace={workspace} app={leftPanelApp} />
						) : (
							<div className="h-full flex items-center justify-center p-6 text-center">
								<div className="flex flex-col items-center">
									<h3 className="m-0 font-medium text-content-primary text-base">
										{leftPanelAppSlug} app not found
									</h3>
									<span className="text-content-secondary text-sm">
										Please, make sure your template has a {leftPanelAppSlug} app
										configured.
									</span>
								</div>
							</div>
						)}
					</Panel>
					<PanelResizeHandle>
						<div className="w-1 bg-border h-full hover:bg-border-hover transition-all relative" />
					</PanelResizeHandle>
					<Panel className="[&>*]:h-full" defaultSize={75}>
						<TaskApps task={task} workspace={workspace} />
					</Panel>
				</PanelGroup>
			);
		}
	}

	return (
		<TaskPageLayout>
			<title>{pageTitle(task.display_name)}</title>

			<TaskTopbar
				task={task}
				workspace={workspace}
				canUpdatePermissions={permissions?.updateWorkspace ?? false}
			/>
			{content}

			<ModifyPromptDialog
				task={task}
				workspace={workspace}
				open={isModifyDialogOpen}
				onOpenChange={setIsModifyDialogOpen}
			/>
		</TaskPageLayout>
	);
};

export default TaskPage;

type WorkspaceNotRunningProps = {
	workspace: Workspace;
	onEditPrompt: () => void;
};

const WorkspaceNotRunning: FC<WorkspaceNotRunningProps> = ({
	workspace,
	onEditPrompt,
}) => {
	const queryClient = useQueryClient();

	const { data: buildParameters } = useQuery(
		workspaceBuildParameters(workspace.latest_build.id),
	);

	const mutateStartWorkspace = useMutation({
		...startWorkspace(workspace, queryClient),
		onError: (error: unknown) => {
			if (!isApiError(error)) {
				displayError(getErrorMessage(error, "Failed to build workspace."));
			}
		},
	});

	// After requesting a workspace start, it may take a while to become ready.
	// Show a loading state in the meantime.
	const isWaitingForStart =
		mutateStartWorkspace.isPending || mutateStartWorkspace.isSuccess;

	const apiError = isApiError(mutateStartWorkspace.error)
		? mutateStartWorkspace.error
		: undefined;

	const deleted = workspace.latest_build?.transition === ("delete" as const);

	return deleted ? (
		<Margins>
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Task workspace was deleted.
					</h3>
					<span className="text-content-secondary text-sm">
						This task cannot be resumed. Delete this task and create a new one.
					</span>
					<Button size="sm" variant="outline" asChild className="mt-4">
						<RouterLink to="/tasks" data-testid="task-create-new">
							Create a new task
						</RouterLink>
					</Button>
				</div>
			</div>
		</Margins>
	) : (
		<Margins>
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Workspace is not running
					</h3>
					<span className="text-content-secondary text-sm">
						Apps and previous statuses are not available
					</span>
					{workspace.outdated && (
						<div
							data-testid="workspace-outdated-tooltip"
							className="flex items-center gap-1.5 mt-1 text-content-secondary text-sm"
						>
							<WorkspaceOutdatedTooltip workspace={workspace}>
								You can update your task workspace to a newer version
							</WorkspaceOutdatedTooltip>
						</div>
					)}
					<div className="flex flex-row mt-4 gap-4">
						<Button
							size="sm"
							data-testid="task-start-workspace"
							disabled={isWaitingForStart}
							onClick={() => {
								mutateStartWorkspace.mutate({
									buildParameters,
								});
							}}
						>
							<Spinner loading={isWaitingForStart} />
							Start workspace
						</Button>
						<Button size="sm" onClick={onEditPrompt} variant="outline">
							Edit Prompt
						</Button>
					</div>
				</div>
			</div>

			<WorkspaceErrorDialog
				open={apiError !== undefined}
				error={apiError}
				onClose={mutateStartWorkspace.reset}
				showDetail={true}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
				isDeleting={false}
			/>
		</Margins>
	);
};

type BuildingWorkspaceProps = {
	workspace: Workspace;
	onEditPrompt: () => void;
};

const BuildingWorkspace: FC<BuildingWorkspaceProps> = ({
	workspace,
	onEditPrompt,
}) => {
	const { data: template } = useQuery(
		templateQueryOptions(workspace.template_id),
	);

	const buildLogs = useWorkspaceBuildLogs(workspace.latest_build.id);

	// If no template yet, use an indeterminate progress bar.
	const transitionStats = (template &&
		getActiveTransitionStats(template, workspace)) || {
		P50: 0,
		P95: null,
	};

	const scrollAreaRef = useRef<HTMLDivElement>(null);
	// biome-ignore lint/correctness/useExhaustiveDependencies: this effect should run when build logs change
	useLayoutEffect(() => {
		if (isChromatic()) {
			return;
		}
		const scrollAreaEl = scrollAreaRef.current;
		const scrollAreaViewportEl = scrollAreaEl?.querySelector<HTMLDivElement>(
			"[data-radix-scroll-area-viewport]",
		);
		if (scrollAreaViewportEl) {
			scrollAreaViewportEl.scrollTop = scrollAreaViewportEl.scrollHeight;
		}
	}, [buildLogs]);

	return (
		<section className="p-16 overflow-y-auto">
			<div className="flex justify-center items-center w-full">
				<div className="flex flex-col gap-6 items-center w-full">
					<header className="flex flex-col items-center text-center">
						<h3 className="m-0 font-medium text-content-primary text-xl">
							Starting your workspace
						</h3>
						<p className="text-content-secondary m-0">
							Your task will be running in a few moments
						</p>
					</header>

					<div className="w-full max-w-screen-lg flex flex-col gap-4 overflow-hidden">
						<WorkspaceBuildProgress
							workspace={workspace}
							transitionStats={transitionStats}
							variant="task"
						/>

						<ScrollArea
							ref={scrollAreaRef}
							className="h-96 border border-solid border-border rounded-lg"
						>
							<WorkspaceBuildLogs
								sticky
								className="border-0 rounded-none"
								logs={buildLogs ?? []}
							/>
						</ScrollArea>

						<div className="flex flex-col items-center gap-3 mt-4">
							<p className="text-content-secondary text-sm m-0 max-w-md text-center">
								You can edit the prompt while we prepare the environment
							</p>
							<Button size="sm" onClick={onEditPrompt}>
								Edit Prompt
							</Button>
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};

type TaskStartingAgentProps = {
	agent: WorkspaceAgent;
};

const TaskStartingAgent: FC<TaskStartingAgentProps> = ({ agent }) => {
	const logs = useAgentLogs({ agentId: agent.id });
	const listRef = useRef<FixedSizeList>(null);

	useLayoutEffect(() => {
		if (listRef.current) {
			listRef.current.scrollToItem(logs.length - 1, "end");
		}
	}, [logs]);

	return (
		<section className="p-16 overflow-y-auto">
			<div className="flex justify-center items-center w-full">
				<div className="flex flex-col gap-8 items-center w-full">
					<header className="flex flex-col items-center text-center">
						<h3 className="m-0 font-medium text-content-primary text-xl">
							Running startup scripts
						</h3>
						<p className="text-content-secondary m-0">
							Your task will be running in a few moments
						</p>
					</header>

					<div className="w-full max-w-screen-lg flex flex-col gap-4 overflow-hidden">
						<div className="h-96 border border-solid border-border rounded-lg">
							<AgentLogs
								ref={listRef}
								sources={agent.log_sources}
								height={96 * 4}
								width="100%"
								overflowed={agent.logs_overflowed}
								logs={logs.map((l) => ({
									id: l.id,
									level: l.level,
									output: l.output,
									sourceId: l.source_id,
									time: l.created_at,
								}))}
							/>
						</div>
					</div>
				</div>
			</div>
		</section>
	);
};

type HeadlessAgentViewProps = {
	task: Task;
	workspace: Workspace;
	canUpdatePermissions: boolean;
};

const HeadlessAgentView: FC<HeadlessAgentViewProps> = ({
	task,
	workspace,
	canUpdatePermissions,
}) => {
	const [isDuplicateDialogOpen, setIsDuplicateDialogOpen] = useState(false);

	// Mock agent activity data for demonstration
	const agentActivity = [
		{
			type: "prompt" as const,
			content:
				"Analyze the codebase and identify potential performance bottlenecks",
			timestamp: new Date(Date.now() - 120000),
		},
		{
			type: "tool_call" as const,
			tool: "code_search",
			args: { pattern: "*.tsx", query: "useEffect" },
			timestamp: new Date(Date.now() - 110000),
		},
		{
			type: "tool_call" as const,
			tool: "read_file",
			args: { path: "src/components/Dashboard.tsx" },
			timestamp: new Date(Date.now() - 100000),
		},
		{
			type: "prompt" as const,
			content: "Check for database query optimization opportunities",
			timestamp: new Date(Date.now() - 90000),
		},
		{
			type: "tool_call" as const,
			tool: "grep",
			args: { pattern: "SELECT \\* FROM", directory: "src/" },
			timestamp: new Date(Date.now() - 80000),
		},
		{
			type: "boundary_blocked" as const,
			reason: "Attempted to access /etc/passwd",
			timestamp: new Date(Date.now() - 70000),
		},
		{
			type: "tool_call" as const,
			tool: "write_file",
			args: { path: "docs/performance-analysis.md", size: "2.3 KB" },
			timestamp: new Date(Date.now() - 60000),
		},
		{
			type: "boundary_blocked" as const,
			reason: "Attempted to execute shell command: rm -rf",
			timestamp: new Date(Date.now() - 50000),
		},
		{
			type: "prompt" as const,
			content: "Generate test coverage report for updated components",
			timestamp: new Date(Date.now() - 40000),
		},
		{
			type: "tool_call" as const,
			tool: "bash",
			args: { command: "npm run test:coverage" },
			timestamp: new Date(Date.now() - 30000),
		},
	];

	return (
		<>
			<div className="flex flex-col h-full">
				{/* Header with metadata */}
				<div className="p-6 border-b border-border bg-surface-secondary">
					<div className="max-w-7xl mx-auto">
						<div className="flex items-start justify-between mb-6">
							<div>
								<div className="flex items-center gap-3 mb-2">
									<img
										src="/icon/tasks.svg"
										alt="Headless Agent"
										className="size-8"
									/>
									<h1 className="m-0 text-2xl font-semibold text-content-primary">
										Headless Agent Session
									</h1>
								</div>
								<p className="m-0 text-sm text-content-secondary">
									Autonomous Mux agent executing tasks in the background
								</p>
							</div>
							<Button
								onClick={() => setIsDuplicateDialogOpen(true)}
								variant="default"
								size="sm"
							>
								<CopyIcon />
								Continue Conversation
							</Button>
						</div>

						{/* Metadata Grid */}
						<div className="grid grid-cols-4 gap-4 p-4 bg-surface-primary border border-border rounded-lg">
							<div>
								<p className="m-0 text-xs font-semibold text-content-secondary mb-1.5">
									Workspace
								</p>
								<p className="m-0 text-sm font-mono text-content-primary">
									{workspace.name}
								</p>
							</div>
							<div>
								<p className="m-0 text-xs font-semibold text-content-secondary mb-1.5">
									Branch
								</p>
								<p className="m-0 text-sm font-mono text-content-primary">
									{workspace.latest_build.template_version_name || "main"}
								</p>
							</div>
							<div>
								<p className="m-0 text-xs font-semibold text-content-secondary mb-1.5">
									Repository
								</p>
								<p className="m-0 text-sm font-mono text-content-primary">
									{workspace.template_name}
								</p>
							</div>
							<div>
								<p className="m-0 text-xs font-semibold text-content-secondary mb-1.5">
									PersistentVolumeClaim
								</p>
								<p className="m-0 text-sm font-mono text-content-primary">
									coder-{workspace.id.slice(0, 8)}-pvc
								</p>
							</div>
						</div>
					</div>
				</div>

				{/* Agent Activity Feed */}
				<ScrollArea className="flex-1">
					<div className="max-w-7xl mx-auto p-6">
						<div className="space-y-3">
							{agentActivity.map((activity, index) => (
								<div
									key={index}
									className={cn(
										"p-4 rounded-lg border",
										activity.type === "prompt" &&
											"bg-surface-secondary border-border",
										activity.type === "tool_call" &&
											"bg-surface-primary border-content-link/20",
										activity.type === "boundary_blocked" &&
											"bg-surface-secondary border-content-destructive",
									)}
								>
									<div className="flex items-start justify-between mb-2">
										<div className="flex items-center gap-2">
											{activity.type === "prompt" && (
												<>
													<SquareTerminalIcon className="size-4 text-content-secondary" />
													<span className="text-xs font-semibold text-content-secondary uppercase">
														Prompt
													</span>
												</>
											)}
											{activity.type === "tool_call" && (
												<>
													<Code2 className="size-4 text-content-link" />
													<span className="text-xs font-semibold text-content-link uppercase">
														Tool Call
													</span>
												</>
											)}
											{activity.type === "boundary_blocked" && (
												<>
													<AlertTriangleIcon className="size-4 text-content-destructive" />
													<span className="text-xs font-semibold text-content-destructive uppercase">
														Blocked by Boundary
													</span>
												</>
											)}
										</div>
										<span className="text-xs text-content-secondary font-mono">
											{activity.timestamp.toLocaleTimeString()}
										</span>
									</div>

									{activity.type === "prompt" && (
										<p className="m-0 text-sm text-content-primary">
											{activity.content}
										</p>
									)}

									{activity.type === "tool_call" && (
										<div className="space-y-1">
											<p className="m-0 text-sm font-semibold text-content-primary font-mono">
												{activity.tool}
											</p>
											<pre className="m-0 text-xs text-content-secondary font-mono bg-surface-secondary p-2 rounded">
												{JSON.stringify(activity.args, null, 2)}
											</pre>
										</div>
									)}

									{activity.type === "boundary_blocked" && (
										<div className="flex items-start gap-2">
											<p className="m-0 text-sm text-content-destructive">
												{activity.reason}
											</p>
										</div>
									)}
								</div>
							))}

							{/* Loading indicator */}
							<div className="p-4 rounded-lg border border-border bg-surface-primary">
								<div className="flex items-center gap-2">
									<Spinner />
									<span className="text-sm text-content-secondary">
										Agent is processing...
									</span>
								</div>
							</div>
						</div>
					</div>
				</ScrollArea>
			</div>

			<NewTaskDialog
				open={isDuplicateDialogOpen}
				onClose={() => setIsDuplicateDialogOpen(false)}
				duplicateMetadata={{
					workspaceName: workspace.name,
					workspaceOwner: workspace.owner_name,
					branch: workspace.latest_build.template_version_name || "main",
					repository: workspace.template_name,
					pvcName: `coder-${workspace.id.slice(0, 8)}-pvc`,
				}}
			/>
		</>
	);
};

function selectAgent(workspace: Workspace) {
	const agents = workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a);

	return agents.at(0);
}
