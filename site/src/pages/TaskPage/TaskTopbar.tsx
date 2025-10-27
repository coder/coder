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
	LaptopMinimalIcon,
	TerminalIcon,
} from "lucide-react";
import type { FC } from "react";
import { Link as RouterLink } from "react-router";
import { TaskStatusLink } from "./TaskStatusLink";

type TaskTopbarProps = { task: Task; workspace: Workspace };

export const TaskTopbar: FC<TaskTopbarProps> = ({ task, workspace }) => {
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

			<h1 className="m-0 pl-2 text-base font-medium truncate">{task.name}</h1>

			{task.current_state?.uri && (
				<div className="flex items-center gap-2 flex-wrap ml-4">
					<TaskStatusLink uri={task.current_state.uri} />
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
								{task.initial_prompt}
							</p>
							<CopyPromptButton prompt={task.initial_prompt} />
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

				<Button asChild variant="outline" size="sm">
					<RouterLink to={`/@${workspace.owner_name}/${workspace.name}`}>
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
