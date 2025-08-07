import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import { template as templateQueryOptions } from "api/queries/templates";
import type {
	Workspace,
	WorkspaceApp,
	WorkspaceStatus,
} from "api/typesGenerated";
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

	const [sidebarApp, sidebarAppStatus] = getSidebarApp(task);

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
		content = <TaskApps task={task} sidebarApp={sidebarApp} />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle(ellipsizeText(task.prompt, 64) ?? "Task")}</title>
			</Helmet>
			<PanelGroup autoSaveId="task" direction="horizontal">
				<Panel defaultSize={25} minSize={20}>
					<TaskSidebar
						task={task}
						sidebarApp={sidebarApp}
						sidebarAppStatus={sidebarAppStatus}
					/>
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

const getSidebarApp = (
	task: Task,
): [WorkspaceApp | null, "error" | "loading" | "healthy"] => {
	if (!task.workspace.latest_build.job.completed_at) {
		// while the workspace build is running, we don't have a sidebar app yet
		return [null, "loading"];
	}

	// Ensure all the agents are healthy before continuing.
	const healthyAgents = task.workspace.latest_build.resources
		.flatMap((res) => res.agents)
		.filter((agt) => !!agt && agt.health.healthy);
	if (!healthyAgents) {
		return [null, "loading"];
	}

	// TODO(Cian): Improve logic for determining sidebar app.
	// For now, we take the first workspace_app with at least one app_status.
	const sidebarApps = healthyAgents
		.flatMap((a) => a?.apps)
		.filter((a) => !!a && a.statuses && a.statuses.length > 0);

	// At this point the workspace build is complete but no app has reported a status
	// indicating that it is ready. Most well-behaved agentic AI applications will
	// indicate their readiness status via MCP(coder_report_task).
	// It's also possible that the application is just not ready yet.
	// We return "loading" instead of "error" to avoid showing an error state if the app
	// becomes available shortly after. The tradeoff is that users may see a loading state
	// indefinitely if there's a genuine issue, but this is preferable to false error alerts.
	if (!sidebarApps) {
		return [null, "loading"];
	}

	const sidebarApp = sidebarApps[0];
	if (!sidebarApp) {
		return [null, "loading"];
	}

	// "disabled" means that the health check is disabled, so we assume
	// that the app is healthy
	if (sidebarApp.health === "disabled") {
		return [sidebarApp, "healthy"];
	}
	if (sidebarApp.health === "healthy") {
		return [sidebarApp, "healthy"];
	}
	if (sidebarApp.health === "initializing") {
		return [sidebarApp, "loading"];
	}
	if (sidebarApp.health === "unhealthy") {
		return [sidebarApp, "error"];
	}

	// exhaustiveness check
	const _: never = sidebarApp.health;
	// this should never happen
	console.error(
		"Task workspace has a finished build but the sidebar app is in an unknown health state",
		task.workspace,
	);
	return [null, "error"];
};
