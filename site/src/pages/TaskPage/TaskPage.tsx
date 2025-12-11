import { API } from "api/api";
import { getErrorDetail, getErrorMessage, isApiError } from "api/errors";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceBuildParameters } from "api/queries/workspaceBuilds";
import {
	startWorkspace,
	workspaceByOwnerAndName,
} from "api/queries/workspaces";
import type {
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
import { ArrowLeftIcon, RotateCcwIcon } from "lucide-react";
import { AgentLogs } from "modules/resources/AgentLogs/AgentLogs";
import { useAgentLogs } from "modules/resources/useAgentLogs";
import { getAllAppsWithAgent } from "modules/tasks/apps";
import { TasksSidebar } from "modules/tasks/TasksSidebar/TasksSidebar";
import { WorkspaceErrorDialog } from "modules/workspaces/ErrorDialog/WorkspaceErrorDialog";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
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
			<title>{pageTitle(task.name)}</title>

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
					<div className="flex flex-row mt-4 gap-4">
						<Button
							size="sm"
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

function selectAgent(workspace: Workspace) {
	const agents = workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.filter((a) => !!a);

	return agents.at(0);
}
