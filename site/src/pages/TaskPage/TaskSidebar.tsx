import type { WorkspaceApp } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowLeftIcon, EllipsisVerticalIcon } from "lucide-react";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { TaskAppIFrame } from "./TaskAppIframe";
import { TaskStatusLink } from "./TaskStatusLink";

type TaskSidebarProps = {
	task: Task;
	sidebarApp: WorkspaceApp | null;
	sidebarAppStatus: "error" | "loading" | "healthy";
};

export const TaskSidebar: FC<TaskSidebarProps> = ({
	task,
	sidebarApp,
	sidebarAppStatus,
}) => {
	return (
		<aside className="flex flex-col h-full shrink-0 w-full">
			<header className="border-0 border-b border-solid border-border p-4 pt-0">
				<div className="flex items-center justify-between py-1">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button size="icon" variant="subtle" asChild className="-ml-2">
									<RouterLink to="/tasks">
										<ArrowLeftIcon />
										<span className="sr-only">Back to tasks</span>
									</RouterLink>
								</Button>
							</TooltipTrigger>
							<TooltipContent>Back to tasks</TooltipContent>
						</Tooltip>
					</TooltipProvider>

					<DropdownMenu>
						<TooltipProvider>
							<Tooltip>
								<TooltipTrigger asChild>
									<DropdownMenuTrigger asChild>
										<Button size="icon" variant="subtle" className="-mr-2">
											<EllipsisVerticalIcon />
											<span className="sr-only">Settings</span>
										</Button>
									</DropdownMenuTrigger>
								</TooltipTrigger>
								<TooltipContent>Settings</TooltipContent>
							</Tooltip>
						</TooltipProvider>

						<DropdownMenuContent>
							<DropdownMenuItem asChild>
								<RouterLink
									to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
								>
									View workspace
								</RouterLink>
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</div>

				<h1 className="m-0 mt-1 text-base font-medium truncate">
					{task.prompt || task.workspace.name}
				</h1>

				{task.workspace.latest_app_status?.uri && (
					<div className="flex items-center gap-2 mt-2 flex-wrap">
						<TaskStatusLink uri={task.workspace.latest_app_status.uri} />
					</div>
				)}
			</header>

			{sidebarAppStatus === "healthy" && sidebarApp ? (
				<TaskAppIFrame
					active
					key={sidebarApp.id}
					app={sidebarApp}
					task={task}
				/>
			) : sidebarAppStatus === "loading" ? (
				<div className="flex-1 flex flex-col items-center justify-center">
					<Spinner loading className="mb-4" />
				</div>
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
