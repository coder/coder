import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ArrowLeftIcon } from "lucide-react";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { TaskStatusLink } from "./TaskStatusLink";

type TaskTopbarProps = { task: Task };

export const TaskTopbar: FC<TaskTopbarProps> = ({ task }) => {
	return (
		<header className="flex items-center px-3 h-14 border-solid border-border border-0 border-b">
			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<Button size="icon" variant="subtle" asChild>
							<RouterLink to="/tasks">
								<ArrowLeftIcon />
								<span className="sr-only">Back to tasks</span>
							</RouterLink>
						</Button>
					</TooltipTrigger>
					<TooltipContent>Back to tasks</TooltipContent>
				</Tooltip>
			</TooltipProvider>

			<h1 className="m-0 text-base font-medium truncate">{task.prompt}</h1>

			{task.workspace.latest_app_status?.uri && (
				<div className="flex items-center gap-2 flex-wrap ml-4">
					<TaskStatusLink uri={task.workspace.latest_app_status.uri} />
				</div>
			)}

			<Button asChild size="sm" variant="outline" className="ml-auto">
				<RouterLink
					to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
				>
					View workspace
				</RouterLink>
			</Button>
		</header>
	);
};
