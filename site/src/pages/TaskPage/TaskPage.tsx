import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type { WorkspaceStatus } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Spinner } from "components/Spinner/Spinner";
import { ArrowLeftIcon, RotateCcwIcon } from "lucide-react";
import { AI_PROMPT_PARAMETER_NAME, type Task } from "modules/tasks/tasks";
import type { ReactNode } from "react";
import { Helmet } from "react-helmet-async";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { Link as RouterLink } from "react-router-dom";
import { ellipsizeText } from "utils/ellipsizeText";
import { pageTitle } from "utils/page";
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
	const waitingStatuses: WorkspaceStatus[] = ["starting", "pending"];
	const terminatedStatuses: WorkspaceStatus[] = [
		"canceled",
		"canceling",
		"deleted",
		"deleting",
		"stopped",
		"stopping",
	];

	if (waitingStatuses.includes(task.workspace.latest_build.status)) {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<Spinner loading className="mb-4" />
					<h3 className="m-0 font-medium text-content-primary text-base">
						Starting your workspace
					</h3>
					<span className="text-content-secondary text-sm">
						This should take a few minutes
					</span>
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
	} else if (terminatedStatuses.includes(task.workspace.latest_build.status)) {
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
	} else if (!task.workspace.latest_app_status) {
		content = (
			<div className="w-full min-h-80 flex items-center justify-center">
				<div className="flex flex-col items-center">
					<Spinner loading className="mb-4" />
					<h3 className="m-0 font-medium text-content-primary text-base">
						Running your task
					</h3>
					<span className="text-content-secondary text-sm">
						The status should be available soon
					</span>
				</div>
			</div>
		);
	} else {
		content = <TaskApps task={task} />;
	}

	return (
		<>
			<Helmet>
				<title>{pageTitle(ellipsizeText(task.prompt, 64)!)}</title>
			</Helmet>

			<div className="h-full flex justify-stretch">
				<TaskSidebar task={task} />
				{content}
			</div>
		</>
	);
};

export default TaskPage;

export const data = {
	fetchTask: async (workspaceOwnerUsername: string, workspaceName: string) => {
		const workspace = await API.getWorkspaceByOwnerAndName(
			workspaceOwnerUsername,
			workspaceName,
		);
		const parameters = await API.getWorkspaceBuildParameters(
			workspace.latest_build.id,
		);
		const prompt = parameters.find(
			(p) => p.name === AI_PROMPT_PARAMETER_NAME,
		)?.value;

		if (!prompt) {
			return;
		}

		return {
			workspace,
			prompt,
		} satisfies Task;
	},
};
