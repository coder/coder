import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { template as templateQueryOptions } from "api/queries/templates";
import type { Workspace, WorkspaceStatus } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import { ArrowLeftIcon, RotateCcwIcon } from "lucide-react";
import { AI_PROMPT_PARAMETER_NAME, type Task } from "modules/tasks/tasks";
import type { ReactNode } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { useParams } from "react-router-dom";
import { Link as RouterLink } from "react-router-dom";
import { ellipsizeText } from "utils/ellipsizeText";
import { pageTitle } from "utils/page";
import {
	ActiveTransition,
	WorkspaceBuildProgress,
} from "../WorkspacePage/WorkspaceBuildProgress";
import { TaskApps } from "./TaskApps";
import { TaskSidebar } from "./TaskSidebar";

const TaskPage = () => {
	const { workspace: workspaceName, username } = useParams() as {
		workspace: string;
		username: string;
	};
	const {
		data: task,
		error,
		refetch,
	} = useQuery({
		queryKey: ["tasks", username, workspaceName],
		queryFn: () => data.fetchTask(username, workspaceName),
		refetchInterval: 5_000,
	});

	const { data: template } = useQuery({
		...templateQueryOptions(task?.workspace.template_id ?? ""),
		enabled: Boolean(task),
	});

	const waitingStatuses: WorkspaceStatus[] = ["starting", "pending"];
	const shouldStreamBuildLogs =
		task && waitingStatuses.includes(task.workspace.latest_build.status);
	const buildLogs = useWorkspaceBuildLogs(
		task?.workspace.latest_build.id ?? "",
		shouldStreamBuildLogs,
	);

	if (error) {
		return (
			<>
				<Helmet>
					<title>{pageTitle("Error loading task")}</title>
				</Helmet>

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
			</>
		);
	}

	if (!task) {
		return (
			<>
				<Helmet>
					<title>{pageTitle("Loading task")}</title>
				</Helmet>
				<Loader fullscreen />
			</>
		);
	}

	let content: ReactNode = null;
	const terminatedStatuses: WorkspaceStatus[] = [
		"canceled",
		"canceling",
		"deleted",
		"deleting",
		"stopped",
		"stopping",
	];

	if (waitingStatuses.includes(task.workspace.latest_build.status)) {
		// If no template yet, use an indeterminate progress bar.
		const transition = (template &&
			ActiveTransition(template, task.workspace)) || { P50: 0, P95: null };
		const lastStage =
			buildLogs?.[buildLogs.length - 1]?.stage || "Waiting for build status";
		content = (
			<div className="w-full min-h-80 flex flex-col">
				<div className="flex flex-col items-center grow justify-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Starting your workspace
					</h3>
					<div className="text-content-secondary text-sm">{lastStage}</div>
				</div>
				<div className="w-full">
					<WorkspaceBuildProgress
						workspace={task.workspace}
						transitionStats={transition}
						variant="task"
					/>
				</div>
			</div>
		);
	} else if (task.workspace.latest_build.status === "failed") {
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
							to={`/@${task.workspace.owner_name}/${task.workspace.name}/builds/${task.workspace.latest_build.build_number}`}
						>
							View logs
						</RouterLink>
					</Button>
				</div>
			</div>
		);
	} else if (task.workspace.latest_build.status !== "running") {
		content = (
			<Margins>
				<div className="w-full min-h-80 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							Workspace is not running
						</h3>
						<span className="text-content-secondary text-sm">
							Apps and previous statuses are not available
						</span>
						<Button size="sm" className="mt-4" asChild>
							<RouterLink
								to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
							>
								View workspace
							</RouterLink>
						</Button>
					</div>
				</div>
			</Margins>
		);
	} else {
		content = <TaskApps task={task} />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle(ellipsizeText(task.prompt, 64) ?? "Task")}</title>
			</Helmet>
			<PanelGroup autoSaveId="task" direction="horizontal">
				<Panel defaultSize={25} minSize={20}>
					<TaskSidebar task={task} />
				</Panel>
				<PanelResizeHandle>
					<div className="w-1 bg-border h-full hover:bg-border-hover transition-all relative" />
				</PanelResizeHandle>
				<Panel className="[&>*]:h-full">{content}</Panel>
			</PanelGroup>
		</>
	);
};

export default TaskPage;

export class WorkspaceDoesNotHaveAITaskError extends Error {
	constructor(workspace: Workspace) {
		super(
			`Workspace ${workspace.owner_name}/${workspace.name} is not running an AI task`,
		);
		this.name = "WorkspaceDoesNotHaveAITaskError";
	}
}

export const data = {
	fetchTask: async (workspaceOwnerUsername: string, workspaceName: string) => {
		const workspace = await API.getWorkspaceByOwnerAndName(
			workspaceOwnerUsername,
			workspaceName,
		);
		if (
			workspace.latest_build.job.completed_at &&
			!workspace.latest_build.has_ai_task
		) {
			throw new WorkspaceDoesNotHaveAITaskError(workspace);
		}

		const parameters = await API.getWorkspaceBuildParameters(
			workspace.latest_build.id,
		);
		const prompt =
			parameters.find((p) => p.name === AI_PROMPT_PARAMETER_NAME)?.value ??
			"Unknown prompt";

		return {
			workspace,
			prompt,
		} satisfies Task;
	},
};
