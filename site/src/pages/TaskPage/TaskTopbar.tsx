import type { Task, Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
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
	LayoutPanelTopIcon,
	SquareTerminalIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { TaskStartupWarningButton } from "./TaskStartupWarningButton";
import { TaskStatusLink } from "./TaskStatusLink";

type TaskTopbarProps = { task: Task; workspace: Workspace };

export const TaskTopbar: FC<TaskTopbarProps> = ({ task, workspace }) => {
	return (
		<header className="flex flex-shrink-0 items-center gap-2 p-3 border-solid border-border border-0 border-b">
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

			<h1 className="m-0 pl-2 text-base font-medium max-w-[520px] truncate">
				{task.display_name}
			</h1>

			{task.current_state?.uri && (
				<div className="flex items-center gap-2 flex-wrap ml-4">
					<TaskStatusLink uri={task.current_state.uri} />
				</div>
			)}

			<div className="ml-auto gap-2 flex items-center">
				<TaskStartupWarningButton
					lifecycleState={task.workspace_agent_lifecycle}
				/>

				<Popover>
					<PopoverTrigger asChild>
						<Button variant="outline" size="sm">
							<SquareTerminalIcon />
							View Prompt
						</Button>
					</PopoverTrigger>
					<PopoverContent
						className="w-[402px] p-4 bg-surface-secondary text-content-secondary text-sm flex flex-col gap-3"
						align="end"
					>
						<div className="text-sm font-semibold text-content-primary">
							Prompt
						</div>
						<div className="m-0 mb-2 select-all leading-snug p-4 border border-solid rounded-lg font-mono">
							<pre className="m-0 whitespace-pre-wrap break-words">
								{task.initial_prompt}
							</pre>
						</div>
						<CopyPromptButton prompt={task.initial_prompt} />
					</PopoverContent>
				</Popover>

				<Button asChild variant="outline" size="sm">
					<RouterLink to={`/@${workspace.owner_name}/${workspace.name}`}>
						<LayoutPanelTopIcon />
						Go to workspace
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
			variant="outline"
			className="p-0 min-w-0 w-full"
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
