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
	DatabaseIcon,
	HardDriveIcon,
	LayoutPanelTopIcon,
	SquareTerminalIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { Link as RouterLink } from "react-router";
import { ShareButton } from "../WorkspacePage/WorkspaceActions/ShareButton";
import { TaskStartupWarningButton } from "./TaskStartupWarningButton";
import { TaskStatusLink } from "./TaskStatusLink";
import { NewTaskDialog } from "modules/tasks/NewTaskDialog/NewTaskDialog";

type TaskTopbarProps = {
	task: Task;
	workspace: Workspace;
	canUpdatePermissions: boolean;
};

export const TaskTopbar: FC<TaskTopbarProps> = ({
	task,
	workspace,
	canUpdatePermissions,
}) => {
	const [isDuplicateDialogOpen, setIsDuplicateDialogOpen] = useState(false);

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

				<TooltipProvider delayDuration={250}>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button variant="outline" size="sm">
								<DatabaseIcon />
								Metadata
							</Button>
						</TooltipTrigger>
						<TooltipContent className="max-w-md bg-surface-secondary p-4">
							<div className="space-y-3">
								<div>
									<p className="m-0 text-xs font-medium text-content-secondary mb-1">
										Branch
									</p>
									<p className="m-0 text-sm text-content-primary font-mono select-all">
										{workspace.latest_build.template_version_name || "main"}
									</p>
								</div>
								<div>
									<p className="m-0 text-xs font-medium text-content-secondary mb-1">
										Repository
									</p>
									<p className="m-0 text-sm text-content-primary font-mono select-all">
										{workspace.template_name}
									</p>
								</div>
								<div>
									<p className="m-0 text-xs font-medium text-content-secondary mb-1">
										<HardDriveIcon className="inline size-3 mr-1" />
										PersistentVolumeClaim
									</p>
									<p className="m-0 text-sm text-content-primary font-mono select-all">
										coder-{workspace.id.slice(0, 8)}-pvc
									</p>
								</div>
								<div className="pt-2 border-t border-border">
									<Button
										variant="subtle"
										size="sm"
										className="p-0 min-w-0"
										asChild
									>
										<RouterLink
											to={`/@${workspace.owner_name}/${workspace.name}/terminal`}
										>
											View session history →
										</RouterLink>
									</Button>
								</div>
							</div>
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

				<ShareButton
					workspace={workspace}
					canUpdatePermissions={canUpdatePermissions}
				/>

				<Button
					onClick={() => setIsDuplicateDialogOpen(true)}
					variant="outline"
					size="sm"
				>
					<CopyIcon />
					Duplicate
				</Button>

				<Button asChild variant="outline" size="sm">
					<RouterLink to={`/@${workspace.owner_name}/${workspace.name}`}>
						<LayoutPanelTopIcon />
						Go to workspace
					</RouterLink>
				</Button>
			</div>

			<NewTaskDialog
				open={isDuplicateDialogOpen}
				onClose={() => setIsDuplicateDialogOpen(false)}
				duplicateMetadata={{
					workspaceName: workspace.name,
					workspaceOwner: workspace.owner_name,
					branch: workspace.latest_build.template_version_name || "main",
					repository: workspace.template_name,
					pvcName: `coder-${workspace.id.slice(0, 8)}-pvc`,
				}}
			/>
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
