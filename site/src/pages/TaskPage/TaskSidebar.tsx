import type { WorkspaceApp } from "api/typesGenerated";
import { Spinner } from "components/Spinner/Spinner";
import { useProxy } from "contexts/ProxyContext";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { TaskAppIFrame } from "./TaskAppIframe";
import { TaskWildcardWarning } from "./TaskWildcardWarning";

type TaskSidebarProps = {
	task: Task;
};

type SidebarAppStatus = "error" | "loading" | "healthy";

const getSidebarApp = (task: Task): [WorkspaceApp | null, SidebarAppStatus] => {
	const sidebarAppId = task.workspace.latest_build.ai_task_sidebar_app_id;
	// a task workspace with a finished build must have a sidebar app id
	if (!sidebarAppId && task.workspace.latest_build.job.completed_at) {
		console.error(
			"Task workspace has a finished build but no sidebar app id",
			task.workspace,
		);
		return [null, "error"];
	}

	const sidebarApp = task.workspace.latest_build.resources
		.flatMap((r) => r.agents)
		.flatMap((a) => a?.apps)
		.find((a) => a?.id === sidebarAppId);

	if (!task.workspace.latest_build.job.completed_at) {
		// while the workspace build is running, we don't have a sidebar app yet
		return [null, "loading"];
	}
	if (!sidebarApp) {
		// The workspace build is complete but the expected sidebar app wasn't found in the resources.
		// This could happen due to timing issues or temporary inconsistencies in the data.
		// We return "loading" instead of "error" to avoid showing an error state if the app
		// becomes available shortly after. The tradeoff is that users may see a loading state
		// indefinitely if there's a genuine issue, but this is preferable to false error alerts.
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

export const TaskSidebar: FC<TaskSidebarProps> = ({ task }) => {
	const proxy = useProxy();

	const [sidebarApp, sidebarAppStatus] = getSidebarApp(task);
	const shouldDisplayWildcardWarning =
		sidebarApp?.subdomain && proxy.proxy?.preferredWildcardHostname === "";

	return (
		<aside className="flex flex-col h-full shrink-0 w-full">
			{sidebarAppStatus === "loading" ? (
				<div className="flex-1 flex flex-col items-center justify-center pb-4">
					<Spinner loading />
				</div>
			) : shouldDisplayWildcardWarning ? (
				<div className="flex-1 flex flex-col items-center justify-center pb-4">
					<TaskWildcardWarning className="max-w-xl" />
				</div>
			) : sidebarAppStatus === "healthy" && sidebarApp ? (
				<TaskAppIFrame
					active
					key={sidebarApp.id}
					app={sidebarApp}
					task={task}
				/>
			) : (
				<div className="flex-1 flex flex-col items-center justify-center">
					<h3 className="m-0 font-medium text-content-primary text-base">
						Error
					</h3>
					<span className="text-content-secondary text-sm">
						<span>Failed to load the sidebar app.</span>
						{sidebarApp?.health != null && (
							<span> The app is {sidebarApp.health}.</span>
						)}
					</span>
				</div>
			)}
		</aside>
	);
};
