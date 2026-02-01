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
	InfoIcon,
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

		// Check if this is a headless workspace by name (case-insensitive)
		const isHeadless = workspace.name?.toLowerCase().includes("headless");
		if (isHeadless) {
			content = (
				<HeadlessAgentView
					task={task}
					workspace={workspace}
					canUpdatePermissions={permissions?.updateWorkspace ?? false}
				/>
			);
		} else {
			// Index 0 uses "code-server", Index 1 uses "mux"
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
	const [selectedFile, setSelectedFile] = useState<string | null>(
		"site/src/pages/TaskPage/TaskTopbar.tsx",
	);

	// Mock agent conversation showing task breakdown
	const agentMessages = [
		{
			type: "agent" as const,
			content:
				"I'll help you add metadata dropdown and duplicate functionality to the task topbar. Let me break this down into subtasks:",
			timestamp: new Date(Date.now() - 300000),
		},
		{
			type: "task_list" as const,
			tasks: [
				"Update TaskTopbar to add Metadata button with dropdown",
				"Add Duplicate button that opens NewTaskDialog",
				"Update NewTaskDialog to accept initialPrompt prop",
				"Test the duplicate functionality",
			],
			timestamp: new Date(Date.now() - 295000),
		},
		{
			type: "agent" as const,
			content: "Starting with task 1: Update TaskTopbar component",
			timestamp: new Date(Date.now() - 290000),
		},
		{
			type: "tool" as const,
			tool: "Read",
			args: "site/src/pages/TaskPage/TaskTopbar.tsx",
			timestamp: new Date(Date.now() - 285000),
		},
		{
			type: "agent" as const,
			content:
				"I can see the current structure. I'll add the Metadata button with a tooltip dropdown showing branch, repository, and PVC information.",
			timestamp: new Date(Date.now() - 280000),
		},
		{
			type: "tool" as const,
			tool: "Edit",
			args: "site/src/pages/TaskPage/TaskTopbar.tsx (lines 87-139)",
			timestamp: new Date(Date.now() - 275000),
		},
		{
			type: "agent" as const,
			content:
				"✓ Completed task 1. Moving to task 2: Add Duplicate button functionality",
			timestamp: new Date(Date.now() - 270000),
		},
		{
			type: "tool" as const,
			tool: "Edit",
			args: "site/src/pages/TaskPage/TaskTopbar.tsx (lines 146-153)",
			timestamp: new Date(Date.now() - 265000),
		},
		{
			type: "agent" as const,
			content:
				"✓ Completed task 2. Moving to task 3: Update NewTaskDialog component",
			timestamp: new Date(Date.now() - 260000),
		},
		{
			type: "tool" as const,
			tool: "Read",
			args: "site/src/modules/tasks/NewTaskDialog/NewTaskDialog.tsx",
			timestamp: new Date(Date.now() - 255000),
		},
		{
			type: "boundary" as const,
			reason: "Attempted to modify .env file - blocked for security",
			timestamp: new Date(Date.now() - 250000),
		},
		{
			type: "agent" as const,
			content:
				"Understood. I'll continue with the NewTaskDialog changes without touching environment files.",
			timestamp: new Date(Date.now() - 245000),
		},
		{
			type: "tool" as const,
			tool: "Edit",
			args: "site/src/modules/tasks/NewTaskDialog/NewTaskDialog.tsx (lines 32-36)",
			timestamp: new Date(Date.now() - 240000),
		},
		{
			type: "agent" as const,
			content: "✓ Completed task 3. Moving to task 4: Testing the changes",
			timestamp: new Date(Date.now() - 235000),
		},
		{
			type: "tool" as const,
			tool: "Bash",
			args: "pnpm check",
			timestamp: new Date(Date.now() - 230000),
		},
		{
			type: "agent" as const,
			content:
				"✓ All tasks completed successfully. The duplicate button now opens the dialog with metadata attached.",
			timestamp: new Date(Date.now() - 225000),
		},
		{
			type: "thinking" as const,
			content: "Planning next steps...",
			timestamp: new Date(Date.now() - 10000),
		},
	];

	// Mock diff data for the right panel
	const fileDiffs = [
		{
			path: "site/src/pages/TaskPage/TaskTopbar.tsx",
			additions: 52,
			deletions: 3,
			diff: `@@ -70,6 +70,58 @@
 			</TooltipContent>
 		</Tooltip>
 	</TooltipProvider>
+
+	<TooltipProvider delayDuration={250}>
+		<Tooltip>
+			<TooltipTrigger asChild>
+				<Button variant="outline" size="sm">
+					<DatabaseIcon />
+					Metadata
+				</Button>
+			</TooltipTrigger>
+			<TooltipContent className="max-w-md bg-surface-secondary p-4">
+				<div className="space-y-3">
+					<div>
+						<p className="m-0 text-xs font-medium text-content-secondary mb-1">
+							Branch
+						</p>
+						<p className="m-0 text-sm text-content-primary font-mono select-all">
+							{workspace.latest_build.template_version_name || "main"}
+						</p>
+					</div>
+				</div>
+			</TooltipContent>
+		</Tooltip>
+	</TooltipProvider>`,
		},
		{
			path: "site/src/modules/tasks/NewTaskDialog/NewTaskDialog.tsx",
			additions: 15,
			deletions: 2,
			diff: `@@ -32,8 +32,9 @@
 type NewTaskDialogProps = {
 	open: boolean;
 	onClose: () => void;
+	initialPrompt?: string;
 };`,
		},
	];

	const selectedDiff = fileDiffs.find((f) => f.path === selectedFile);

	return (
		<>
			<PanelGroup direction="horizontal">
				{/* Left: Chat-like conversation */}
				<Panel defaultSize={50} minSize={30}>
					<div className="flex flex-col h-full bg-surface-primary">
						{/* Compact header */}
						<div className="px-4 py-3 border-b border-border bg-surface-secondary flex items-center justify-between">
							<div className="flex items-center gap-2">
								<img
									src="/icon/tasks.svg"
									alt="Headless Agent"
									className="size-5 opacity-60"
								/>
								<div>
									<h2 className="m-0 text-sm font-semibold text-content-primary">
										Headless Session
									</h2>
									<p className="m-0 text-xs text-content-secondary">
										No IDE, just pure agent execution. Watch the Mux agent work
										autonomously.
									</p>
								</div>
							</div>
							<Button
								onClick={() => setIsDuplicateDialogOpen(true)}
								variant="outline"
								size="sm"
							>
								<CopyIcon />
								Continue in a new session
							</Button>
						</div>

						{/* Chat messages */}
						<ScrollArea className="flex-1">
							<div className="p-4 space-y-3 max-w-3xl">
								{agentMessages.map((message, index) => (
									<div key={index}>
										{message.type === "agent" && (
											<div className="flex items-start gap-3">
												<div className="flex-shrink-0 size-6 rounded-full bg-content-link/10 flex items-center justify-center mt-0.5">
													<img
														src="/icon/coder.svg"
														alt=""
														className="size-4"
													/>
												</div>
												<div className="flex-1 min-w-0">
													<p className="m-0 text-sm text-content-primary leading-relaxed">
														{message.content}
													</p>
													<span className="text-xs text-content-secondary mt-1 inline-block">
														{message.timestamp.toLocaleTimeString()}
													</span>
												</div>
											</div>
										)}

										{message.type === "task_list" && (
											<div className="ml-9 p-3 bg-surface-secondary border border-border rounded-lg">
												<p className="m-0 text-xs font-semibold text-content-secondary mb-2 uppercase">
													Task Breakdown
												</p>
												<ul className="m-0 space-y-1.5 list-none p-0">
													{message.tasks.map((task, i) => (
														<li
															key={i}
															className="text-sm text-content-primary flex items-start gap-2"
														>
															<span className="text-content-secondary">
																{i + 1}.
															</span>
															<span>{task}</span>
														</li>
													))}
												</ul>
											</div>
										)}

										{message.type === "tool" && (
											<div className="ml-9">
												<div className="inline-flex items-center gap-1.5 px-2 py-1 bg-surface-secondary border border-border rounded text-xs">
													<Code2 className="size-3 text-content-secondary" />
													<span className="text-content-secondary">
														{message.tool}
													</span>
													<span className="text-content-primary font-mono">
														{message.args}
													</span>
												</div>
											</div>
										)}

										{message.type === "boundary" && (
											<div className="ml-9 p-3 bg-surface-secondary border border-content-destructive/30 rounded-lg flex items-start gap-2">
												<AlertTriangleIcon className="size-4 text-content-destructive flex-shrink-0 mt-0.5" />
												<div>
													<p className="m-0 text-xs font-semibold text-content-destructive mb-1">
														Security Boundary
													</p>
													<p className="m-0 text-sm text-content-secondary">
														{message.reason}
													</p>
												</div>
											</div>
										)}

										{message.type === "thinking" && (
											<div className="flex items-start gap-3">
												<div className="flex-shrink-0 size-6 rounded-full bg-content-link/10 flex items-center justify-center mt-0.5">
													<Spinner className="size-3" />
												</div>
												<div className="flex-1 min-w-0">
													<p className="m-0 text-sm text-content-secondary italic">
														{message.content}
													</p>
												</div>
											</div>
										)}
									</div>
								))}
							</div>
						</ScrollArea>

						{/* Metadata footer */}
						<div className="px-4 py-2 border-t border-border bg-surface-secondary">
							<div className="flex items-center justify-between text-xs">
								<div className="flex items-center gap-4">
									<span className="text-content-secondary">
										<span className="font-semibold">Branch:</span>{" "}
										<span className="font-mono">
											{workspace.latest_build.template_version_name || "main"}
										</span>
									</span>
									<span className="text-content-secondary">
										<span className="font-semibold">PVC:</span>{" "}
										<span className="font-mono">
											coder-{workspace.id.slice(0, 8)}
										</span>
									</span>
								</div>
								<span className="text-content-secondary">Mux Agent</span>
							</div>
						</div>
					</div>
				</Panel>

				<PanelResizeHandle>
					<div className="w-1 bg-border h-full hover:bg-border-hover transition-all relative" />
				</PanelResizeHandle>

				{/* Right: Diff viewer */}
				<Panel defaultSize={50} minSize={30}>
					<div className="flex flex-col h-full bg-surface-primary">
						{/* File tabs */}
						<div className="border-b border-border bg-surface-secondary">
							<div className="flex items-center gap-1 px-2 py-1 overflow-x-auto">
								{fileDiffs.map((file) => (
									<button
										key={file.path}
										type="button"
										onClick={() => setSelectedFile(file.path)}
										className={cn(
											"px-3 py-1.5 text-xs font-mono rounded-t transition-colors whitespace-nowrap",
											selectedFile === file.path
												? "bg-surface-primary text-content-primary"
												: "text-content-secondary hover:text-content-primary hover:bg-surface-tertiary",
										)}
									>
										<span className="truncate max-w-[200px] inline-block">
											{file.path.split("/").pop()}
										</span>
										<span className="ml-2 text-green-500">
											+{file.additions}
										</span>
										<span className="ml-1 text-red-500">-{file.deletions}</span>
									</button>
								))}
							</div>
						</div>

						{/* Diff content */}
						<ScrollArea className="flex-1">
							{selectedDiff && (
								<div className="p-4">
									<div className="mb-3 flex items-center justify-between">
										<p className="m-0 text-xs text-content-secondary font-mono">
											{selectedDiff.path}
										</p>
										<div className="flex items-center gap-2 text-xs">
											<span className="text-green-500">
												+{selectedDiff.additions} additions
											</span>
											<span className="text-red-500">
												-{selectedDiff.deletions} deletions
											</span>
										</div>
									</div>
									<pre className="m-0 text-xs font-mono bg-surface-secondary border border-border rounded-lg p-4 overflow-x-auto">
										<code className="text-content-primary whitespace-pre">
											{selectedDiff.diff}
										</code>
									</pre>
								</div>
							)}
						</ScrollArea>
					</div>
				</Panel>
			</PanelGroup>

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
