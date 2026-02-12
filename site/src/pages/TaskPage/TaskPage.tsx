import { API } from "api/api";
import { getErrorDetail, getErrorMessage, isApiError } from "api/errors";
import { pauseTask, resumeTask, taskLogs } from "api/queries/tasks";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import type {
	Task,
	TaskLogEntry,
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
	ArrowLeftIcon,
	PauseIcon,
	RotateCcwIcon,
	TriangleAlertIcon,
} from "lucide-react";
import { AgentLogs } from "modules/resources/AgentLogs/AgentLogs";
import { useAgentLogs } from "modules/resources/useAgentLogs";
import { getAllAppsWithAgent } from "modules/tasks/apps";
import { TasksSidebar } from "modules/tasks/TasksSidebar/TasksSidebar";
import { isPauseDisabled } from "modules/tasks/taskActions";
import { WorkspaceErrorDialog } from "modules/workspaces/ErrorDialog/WorkspaceErrorDialog";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import { WorkspaceOutdatedTooltip } from "modules/workspaces/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import {
	type DependencyList,
	type FC,
	type PropsWithChildren,
	type ReactNode,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { Link as RouterLink, useParams } from "react-router";
import type { FixedSizeList } from "react-window";
import { pageTitle } from "utils/page";
import {
	getActiveTransitionStats,
	WorkspaceBuildProgress,
} from "../WorkspacePage/WorkspaceBuildProgress";
import { ModifyPromptDialog } from "./ModifyPromptDialog";
import { TaskAppIFrame } from "./TaskAppIframe";
import { TaskApps } from "./TaskApps";
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
	const { data: workspace, ...workspaceQuery } = useQuery({
		...workspaceByOwnerAndName(username, task?.workspace_name ?? ""),
		enabled: task !== undefined,
		refetchInterval: ({ state }) => {
			return state.error ? false : 5_000;
		},
	});
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
		content = <TaskBuildFailed task={task} workspace={workspace} />;
	} else if (workspace.latest_build.status === "stopping") {
		content = (
			<TaskTransitioning
				title="Pausing task"
				subtitle="Your task is being paused..."
			/>
		);
	} else if (
		workspace.latest_build.status === "stopped" ||
		workspace.latest_build.status === "canceled"
	) {
		content = (
			<TaskPaused
				task={task}
				workspace={workspace}
				onEditPrompt={() => setIsModifyDialogOpen(true)}
			/>
		);
	} else if (workspace.latest_build.status === "canceling") {
		content = (
			<TaskTransitioning
				title="Canceling task"
				subtitle="Your task is being canceled..."
			/>
		);
	} else if (workspace.latest_build.status === "deleting") {
		content = (
			<TaskTransitioning
				title="Deleting task"
				subtitle="Your task workspace is being deleted..."
			/>
		);
	} else if (workspace.latest_build.status === "deleted") {
		content = <TaskDeleted />;
	} else if (agent && ["created", "starting"].includes(agent.lifecycle_state)) {
		content = <TaskStartingAgent task={task} agent={agent} />;
	} else {
		const chatApp = getAllAppsWithAgent(workspace).find(
			(app) => app.id === task.workspace_app_id,
		);
		content = (
			<PanelGroup autoSaveId="task" direction="horizontal">
				<Panel defaultSize={25} minSize={20}>
					{chatApp ? (
						<TaskAppIFrame active workspace={workspace} app={chatApp} />
					) : (
						<div className="h-full flex items-center justify-center p-6 text-center">
							<div className="flex flex-col items-center">
								<h3 className="m-0 font-medium text-content-primary text-base">
									Chat app not found
								</h3>
								<span className="text-content-secondary text-sm">
									Please, make sure your template has a chat sidebar app
									configured.
								</span>
							</div>
						</div>
					)}
				</Panel>
				<PanelResizeHandle>
					<div className="w-1 bg-border h-full hover:bg-border-hover transition-all relative" />
				</PanelResizeHandle>
				<Panel className="[&>*]:h-full">
					<TaskApps task={task} workspace={workspace} />
				</Panel>
			</PanelGroup>
		);
	}

	return (
		<TaskPageLayout>
			<title>{pageTitle(task.display_name)}</title>

			<TaskTopbar task={task} workspace={workspace} />
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

/**
 * Common component for task state messages (paused, deleted, transitioning, etc.)
 * Similar to EmptyState but styled for task states.
 */
type TaskStateMessageProps = {
	title: string;
	description?: string;
	icon?: ReactNode;
	actions?: ReactNode;
	detail?: ReactNode;
};

const TaskStateMessage: FC<TaskStateMessageProps> = ({
	title,
	description,
	icon,
	actions,
	detail,
}) => {
	return (
		<Margins>
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center text-center">
					<h3 className="m-0 font-medium text-content-primary text-base flex items-center gap-2">
						{icon}
						{title}
					</h3>
					{description && (
						<span className="text-content-secondary text-sm">
							{description}
						</span>
					)}
					{detail}
					{actions && <div className="mt-4">{actions}</div>}
				</div>
			</div>
		</Margins>
	);
};

type TaskTransitioningProps = {
	title: string;
	subtitle: string;
};

const TaskTransitioning: FC<TaskTransitioningProps> = ({ title, subtitle }) => {
	return (
		<TaskStateMessage
			title={title}
			description={subtitle}
			icon={<Spinner loading />}
		/>
	);
};

const TaskDeleted: FC = () => {
	return (
		<TaskStateMessage
			title="Task was deleted"
			description="This task cannot be resumed. Create a new task to continue."
			actions={
				<Button size="sm" variant="outline" asChild>
					<RouterLink to="/tasks" data-testid="task-create-new">
						Create a new task
					</RouterLink>
				</Button>
			}
		/>
	);
};

/**
 * Auto-scrolls a ScrollArea to the bottom whenever deps change.
 * Shared by BuildingWorkspace (build logs) and TaskLogPreview (chat logs).
 */
function useScrollAreaAutoScroll(deps: DependencyList) {
	const scrollAreaRef = useRef<HTMLDivElement>(null);

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
		// biome-ignore lint/correctness/useExhaustiveDependencies: caller controls deps
	}, deps);

	return scrollAreaRef;
}

type TaskLogPreviewProps = {
	logs: readonly TaskLogEntry[];
	maxLines?: number;
	headerAction?: ReactNode;
};

const TaskLogPreview: FC<TaskLogPreviewProps> = ({
	logs,
	maxLines = 30,
	headerAction,
}) => {
	const lines = logs.flatMap((entry) => entry.content.split("\n"));
	const visibleLines = lines.slice(-maxLines);
	const scrollAreaRef = useScrollAreaAutoScroll([visibleLines]);

	return (
		<div className="w-full max-w-screen-lg flex flex-col gap-2">
			<div className="flex items-center justify-between text-sm text-content-secondary px-1">
				<span>Last {maxLines} lines of AI chat logs</span>
				{headerAction}
			</div>
			<ScrollArea
				ref={scrollAreaRef}
				className="h-96 border border-solid border-border rounded-lg"
			>
				<div className="p-4 font-mono text-xs text-content-secondary leading-relaxed whitespace-pre-wrap break-words">
					{visibleLines.map((line, i) => (
						<div key={i}>{line || "\u00A0"}</div>
					))}
				</div>
			</ScrollArea>
		</div>
	);
};

type TaskBuildFailedProps = {
	task: Task;
	workspace: Workspace;
};

const TaskBuildFailed: FC<TaskBuildFailedProps> = ({ task, workspace }) => {
	const { data: logsData } = useQuery({
		...taskLogs(task.owner_name, task.id),
		retry: false,
	});

	const buildLogsLink = `/@${workspace.owner_name}/${workspace.name}/builds/${workspace.latest_build.build_number}`;
	const hasLogs = logsData && logsData.logs.length > 0;

	return (
		<>
			<TaskStateMessage
				title="Task build failed"
				description="Please check the logs for more details."
				icon={<TriangleAlertIcon className="size-4 text-content-destructive" />}
				actions={
					<Button size="sm" variant="outline" asChild>
						<RouterLink to={buildLogsLink}>View full logs</RouterLink>
					</Button>
				}
			/>
			{hasLogs && (
				<TaskLogPreview
					logs={logsData.logs}
					headerAction={
						<Button size="sm" variant="subtle" asChild>
							<RouterLink to={buildLogsLink}>View full logs</RouterLink>
						</Button>
					}
				/>
			)}
		</>
	);
};

type TaskPausedProps = {
	task: Task;
	workspace: Workspace;
	onEditPrompt: () => void;
};

const TaskPaused: FC<TaskPausedProps> = ({ task, workspace, onEditPrompt }) => {
	const queryClient = useQueryClient();

	// Use mutation config directly to customize error handling:
	// API errors are shown in a dialog, other errors show a toast.
	const resumeMutation = useMutation({
		...resumeTask(task, queryClient),
		onError: (error: unknown) => {
			if (!isApiError(error)) {
				displayError(getErrorMessage(error, "Failed to resume task."));
			}
		},
	});

	const { data: logsData } = useQuery({
		...taskLogs(task.owner_name, task.id),
		retry: false,
	});

	// After requesting a task resume, it may take a while to become ready.
	const isWaitingForStart =
		resumeMutation.isPending || resumeMutation.isSuccess;

	// Determine if this was a timeout (autostop) or manual pause.
	const isTimeout = workspace.latest_build.reason === "autostop";

	const apiError = isApiError(resumeMutation.error)
		? resumeMutation.error
		: undefined;

	const hasLogs = logsData && logsData.logs.length > 0;

	return (
		<>
			<TaskStateMessage
				title="Task paused"
				description={
					isTimeout
						? "Your task timed out. Resume it to continue."
						: "Resume the task to continue."
				}
				icon={<PauseIcon className="size-4" />}
				detail={
					workspace.outdated && (
						<div
							data-testid="workspace-outdated-tooltip"
							className="flex items-center gap-1.5 mt-1 text-content-secondary text-sm"
						>
							<WorkspaceOutdatedTooltip workspace={workspace}>
								A newer template version is available
							</WorkspaceOutdatedTooltip>
						</div>
					)
				}
				actions={
					<div className="flex flex-row gap-4">
						<Button
							size="sm"
							disabled={isWaitingForStart}
							onClick={() => resumeMutation.mutate()}
						>
							<Spinner loading={isWaitingForStart} />
							Resume
						</Button>
						<Button size="sm" onClick={onEditPrompt} variant="outline">
							Edit prompt
						</Button>
					</div>
				}
			/>
			{hasLogs && (
				<TaskLogPreview
					logs={logsData.logs}
					headerAction={
						<Button
							size="sm"
							variant="subtle"
							disabled={isWaitingForStart}
							onClick={() => resumeMutation.mutate()}
						>
							<Spinner loading={isWaitingForStart} />
							Resume to view full logs
						</Button>
					}
				/>
			)}

			<WorkspaceErrorDialog
				open={apiError !== undefined}
				error={apiError}
				onClose={resumeMutation.reset}
				showDetail={true}
				workspaceOwner={workspace.owner_name}
				workspaceName={workspace.name}
				templateVersionId={workspace.latest_build.template_version_id}
				isDeleting={false}
			/>
		</>
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

	const scrollAreaRef = useScrollAreaAutoScroll([buildLogs]);

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
	task: Task;
	agent: WorkspaceAgent;
};

const TaskStartingAgent: FC<TaskStartingAgentProps> = ({ task, agent }) => {
	const logs = useAgentLogs({ agentId: agent.id });
	const listRef = useRef<FixedSizeList>(null);
	const queryClient = useQueryClient();
	const pauseMutation = useMutation({
		...pauseTask(task, queryClient),
		onError: (error: unknown) => {
			displayError(getErrorMessage(error, "Failed to pause task."));
		},
	});
	const pauseDisabled = isPauseDisabled(task.status);

	useLayoutEffect(() => {
		if (listRef.current) {
			listRef.current.scrollToItem(logs.length - 1, "end");
		}
	}, [logs]);

	return (
		<section className="p-16 overflow-y-auto">
			<div className="flex justify-center items-center w-full">
				<div className="flex flex-col gap-8 items-center w-full">
					<header className="flex flex-col items-center text-center gap-3">
						<div>
							<h3 className="m-0 font-medium text-content-primary text-xl">
								Running startup scripts
							</h3>
							<p className="text-content-secondary m-0">
								Your task will be running in a few moments
							</p>
						</div>
						<Button
							size="sm"
							variant="outline"
							disabled={pauseDisabled || pauseMutation.isPending}
							onClick={() => pauseMutation.mutate()}
						>
							<Spinner loading={pauseMutation.isPending}>
								<PauseIcon className="size-4" />
							</Spinner>
							Pause
						</Button>
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

function selectAgent(workspace: Workspace) {
	const agents = workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a);

	return agents.at(0);
}
