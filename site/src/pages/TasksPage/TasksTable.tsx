import { getErrorDetail, getErrorMessage } from "api/errors";
import type { Task } from "api/typesGenerated";
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
import { TaskStatus } from "modules/tasks/TaskStatus/TaskStatus";
import { type FC, type ReactNode, useState } from "react";
import { Link as RouterLink } from "react-router";
import { relativeTime } from "utils/time";

type TasksTableProps = {
	tasks: readonly Task[] | undefined;
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
		body = tasks.map((task) => <TaskRow key={task.id} task={task} />);
	}

	return (
		<Table className="mt-4">
			<TableHeader>
				<TableRow>
					<TableHead>Task</TableHead>
					<TableHead>Status</TableHead>
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
			<TableCell colSpan={999} className="text-center">
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
			<TableCell colSpan={999} className="text-center">
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
	const templateDisplayName = task.template_display_name ?? task.template_name;

	return (
		<>
			<TableRow className="relative" hover>
				<TableCell>
					<AvatarData
						title={
							<>
								<span className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap">
									{task.initial_prompt}
								</span>
								<RouterLink
									to={`/tasks/${task.owner_name}/${task.id}`}
									className="absolute inset-0"
								>
									<span className="sr-only">Access task</span>
								</RouterLink>
							</>
						}
						subtitle={templateDisplayName}
						avatar={
							<Avatar
								size="lg"
								variant="icon"
								src={task.template_icon}
								fallback={templateDisplayName}
							/>
						}
					/>
				</TableCell>
				<TableCell>
					<TaskStatus
						status={task.status}
						stateMessage={task.current_state?.message || "No message available"}
					/>
				</TableCell>

				<TableCell>
					<AvatarData
						title={task.owner_name}
						subtitle={
							<span className="block first-letter:uppercase">
								{relativeTime(new Date(task.created_at))}
							</span>
						}
						src={task.owner_avatar_url}
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
