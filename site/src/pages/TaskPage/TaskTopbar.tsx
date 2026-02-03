import type { Task, Workspace } from "api/typesGenerated";
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

				<TooltipProvider delayDuration={250}>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button variant="outline" size="sm">
								<SquareTerminalIcon />
								View Prompt
							</Button>
						</TooltipTrigger>
						<TooltipContent className="max-w-xs bg-surface-secondary p-4">
							<p className="m-0 mb-2 select-all text-sm font-normal text-content-primary leading-snug">
								{task.initial_prompt}
							</p>
							<CopyPromptButton prompt={task.initial_prompt} />
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

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
