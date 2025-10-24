import { getErrorDetail, getErrorMessage } from "api/errors";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { Button } from "components/Button/Button";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { RotateCcwIcon, TrashIcon } from "lucide-react";
import { TaskDeleteDialog } from "modules/tasks/TaskDeleteDialog/TaskDeleteDialog";
import { getCleanTaskName, type Task } from "modules/tasks/tasks";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import { WorkspaceStatus } from "modules/workspaces/WorkspaceStatus/WorkspaceStatus";
import { type FC, type ReactNode, useState } from "react";
import { Link as RouterLink } from "react-router";
import { relativeTime } from "utils/time";

type TasksTableProps = {
	tasks: Task[] | undefined;
	error: unknown;
	onRetry: () => void;
};

export const TasksTable: FC<TasksTableProps> = ({ tasks, error, onRetry }) => {
	let body: ReactNode = null;

	if (error) {
		body = <TasksErrorBody error={error} onRetry={onRetry} />;
	} else if (!tasks) {
		body = <TasksSkeleton />;
	} else if (tasks.length === 0) {
		body = <TasksEmpty />;
	} else {
		body = tasks.map((task) => <TaskRow key={task.workspace.id} task={task} />);
	}

	return (
		<Table className="mt-4">
			<TableHeader>
				<TableRow>
					<TableHead>Task</TableHead>
					<TableHead>Agent status</TableHead>
					<TableHead>Workspace status</TableHead>
					<TableHead>Created by</TableHead>
					<TableHead />
				</TableRow>
			</TableHeader>
			<TableBody>{body}</TableBody>
		</Table>
	);
};

type TasksErrorBodyProps = {
	error: unknown;
	onRetry: () => void;
};

const TasksErrorBody: FC<TasksErrorBodyProps> = ({ error, onRetry }) => {
	return (
		<TableRow>
			<TableCell colSpan={5} className="text-center">
				<div className="rounded-lg w-full min-h-80 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							{getErrorMessage(error, "Error loading tasks")}
						</h3>
						<span className="text-content-secondary text-sm">
							{getErrorDetail(error) ?? "Please try again"}
						</span>
						<Button size="sm" onClick={onRetry} className="mt-4">
							<RotateCcwIcon />
							Try again
						</Button>
					</div>
				</div>
			</TableCell>
		</TableRow>
	);
};

const TasksEmpty: FC = () => {
	return (
		<TableRow>
			<TableCell colSpan={5} className="text-center">
				<div className="w-full min-h-80 p-4 flex items-center justify-center">
					<div className="flex flex-col items-center">
						<h3 className="m-0 font-medium text-content-primary text-base">
							No tasks found
						</h3>
						<span className="text-content-secondary text-sm">
							Use the form above to run a task
						</span>
					</div>
				</div>
			</TableCell>
		</TableRow>
	);
};

type TaskRowProps = { task: Task };

const TaskRow: FC<TaskRowProps> = ({ task }) => {
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
	const templateDisplayName =
		task.workspace.template_display_name ?? task.workspace.template_name;
	const cleanTaskName = getCleanTaskName(task.workspace.name);
	const truncatedPrompt =
		task.prompt.length > 100 ? `${task.prompt.slice(0, 100)}...` : task.prompt;

	return (
		<>
			<TableRow key={task.workspace.id} className="relative" hover>
				<TableCell>
					<RouterLink
						to={`/tasks/${task.workspace.owner_name}/${task.workspace.name}`}
						className="absolute inset-0"
					>
						<span className="sr-only">Access task</span>
					</RouterLink>
					<AvatarData
						title={
							<TooltipProvider>
								<Tooltip>
									<TooltipTrigger asChild>
										<RouterLink
											to={`/tasks/${task.workspace.owner_name}/${task.workspace.name}`}
											className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap cursor-help relative z-10 no-underline text-inherit"
										>
											{cleanTaskName}
										</RouterLink>
									</TooltipTrigger>
									<TooltipContent className="max-w-md">
										<div className="space-y-2">
											<div>
												<div className="text-xs text-content-secondary">
													Workspace name
												</div>
												<div className="text-sm font-medium">
													{task.workspace.name}
												</div>
											</div>
											<div>
												<div className="text-xs text-content-secondary">
													Prompt
												</div>
												<div className="text-sm">{truncatedPrompt}</div>
											</div>
										</div>
									</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						}
						subtitle={templateDisplayName}
						avatar={
							<Avatar
								size="lg"
								variant="icon"
								src={task.workspace.template_icon}
								fallback={templateDisplayName}
							/>
						}
					/>
				</TableCell>
				<TableCell>
					<WorkspaceAppStatus
						disabled={task.workspace.latest_build.status !== "running"}
						status={task.workspace.latest_app_status}
					/>
				</TableCell>
				<TableCell>
					<WorkspaceStatus workspace={task.workspace} />
				</TableCell>
				<TableCell>
					<AvatarData
						title={task.workspace.owner_name}
						subtitle={
							<span className="block first-letter:uppercase">
								{relativeTime(new Date(task.workspace.created_at))}
							</span>
						}
						src={task.workspace.owner_avatar_url}
					/>
				</TableCell>
				<TableCell className="text-right">
					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									size="icon"
									variant="outline"
									className="relative z-50"
									onClick={() => setIsDeleteDialogOpen(true)}
								>
									<span className="sr-only">Delete task</span>
									<TrashIcon />
								</Button>
							</TooltipTrigger>
							<TooltipContent>Delete task</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</TableCell>
			</TableRow>

			<TaskDeleteDialog
				task={task}
				open={isDeleteDialogOpen}
				onClose={() => {
					setIsDeleteDialogOpen(false);
				}}
			/>
		</>
	);
};

const TasksSkeleton: FC = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<AvatarDataSkeleton />
				</TableCell>
				<TableCell>
					<Skeleton className="w-[100px] h-6" />
				</TableCell>
				<TableCell>
					<Skeleton className="w-[100px] h-6" />
				</TableCell>
				<TableCell>
					<AvatarDataSkeleton />
				</TableCell>
				<TableCell>
					<div className="flex justify-end items-center">
						<Skeleton className="size-8" />
					</div>
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
