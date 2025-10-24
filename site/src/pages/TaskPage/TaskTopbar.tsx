import { Button } from "components/Button/Button";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useClipboard } from "hooks";
import {
	ArrowLeftIcon,
	CheckIcon,
	CopyIcon,
	LaptopMinimalIcon,
	TerminalIcon,
} from "lucide-react";
import { getCleanTaskName, type Task } from "modules/tasks/tasks";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { TaskStatusLink } from "./TaskStatusLink";

type TaskTopbarProps = { task: Task };

export const TaskTopbar: FC<TaskTopbarProps> = ({ task }) => {
	const cleanTaskName = getCleanTaskName(task.workspace.name);
	const truncatedPrompt =
		task.prompt.length > 100 ? `${task.prompt.slice(0, 100)}...` : task.prompt;

	return (
		<header className="flex flex-shrink-0 items-center p-3 border-solid border-border border-0 border-b">
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

			<TooltipProvider>
				<Tooltip>
					<TooltipTrigger asChild>
						<h1 className="m-0 pl-2 text-base font-medium truncate cursor-help">
							{cleanTaskName}
						</h1>
					</TooltipTrigger>
					<TooltipContent className="max-w-md">
						<div className="space-y-2">
							<div>
								<div className="text-xs text-content-secondary">
									Workspace name
								</div>
								<div className="text-sm font-medium">{task.workspace.name}</div>
							</div>
							<div>
								<div className="text-xs text-content-secondary">Prompt</div>
								<div className="text-sm">{truncatedPrompt}</div>
							</div>
						</div>
					</TooltipContent>
				</Tooltip>
			</TooltipProvider>

			{task.workspace.latest_app_status?.uri && (
				<div className="flex items-center gap-2 flex-wrap ml-4">
					<TaskStatusLink uri={task.workspace.latest_app_status.uri} />
				</div>
			)}

			<div className="ml-auto gap-2 flex items-center">
				<TooltipProvider delayDuration={250}>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button variant="outline" size="sm">
								<TerminalIcon />
								Prompt
							</Button>
						</TooltipTrigger>
						<TooltipContent className="max-w-xs bg-surface-secondary p-4">
							<p className="m-0 mb-2 select-all text-sm font-normal text-content-primary leading-snug">
								{task.prompt}
							</p>
							<CopyPromptButton prompt={task.prompt} />
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

				<Button asChild variant="outline" size="sm">
					<RouterLink
						to={`/@${task.workspace.owner_name}/${task.workspace.name}`}
					>
						<LaptopMinimalIcon />
						Workspace
					</RouterLink>
				</Button>
			</div>
		</header>
	);
};

type CopyPromptButtonProps = { prompt: string };

const CopyPromptButton: FC<CopyPromptButtonProps> = ({ prompt }) => {
	const { copyToClipboard, showCopiedSuccess } = useClipboard();
	return (
		<Button
			disabled={showCopiedSuccess}
			onClick={() => copyToClipboard(prompt)}
			size="sm"
			variant="subtle"
			className="p-0 min-w-0"
		>
			{showCopiedSuccess ? (
				<>
					<CheckIcon /> Copied!
				</>
			) : (
				<>
					<CopyIcon /> Copy
				</>
			)}
		</Button>
	);
};
