import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type { Task } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "components/Avatar/AvatarDataSkeleton";
import { Button } from "components/Button/Button";
import { Checkbox } from "components/Checkbox/Checkbox";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { displayError } from "components/GlobalSnackbar/utils";
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
import { useClickableTableRow } from "hooks";
import { EllipsisVertical, RotateCcwIcon, TrashIcon } from "lucide-react";
import { TaskDeleteDialog } from "modules/tasks/TaskDeleteDialog/TaskDeleteDialog";
import { TaskStatus } from "modules/tasks/TaskStatus/TaskStatus";
import { type FC, type ReactNode, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { relativeTime } from "utils/time";
import { TaskActionButton } from "./TaskActionButton";

type TasksTableProps = {
	tasks: readonly Task[] | undefined;
	error: unknown;
	onRetry: () => void;
	checkedTaskIds?: Set<string>;
	onCheckChange?: (checkedTaskIds: Set<string>) => void;
	canCheckTasks?: boolean;
};

export const TasksTable: FC<TasksTableProps> = ({
	tasks,
	error,
	onRetry,
	checkedTaskIds = new Set(),
	onCheckChange,
	canCheckTasks = false,
}) => {
	let body: ReactNode = null;

	if (error) {
		body = <TasksErrorBody error={error} onRetry={onRetry} />;
	} else if (!tasks) {
		body = <TasksSkeleton canCheckTasks={canCheckTasks} />;
	} else if (tasks.length === 0) {
		body = <TasksEmpty />;
	} else {
		body = tasks.map((task) => {
			const checked = checkedTaskIds.has(task.id);
			return (
				<TaskRow
					key={task.id}
					task={task}
					checked={checked}
					onCheckChange={(taskId, checked) => {
						if (!onCheckChange) return;
						const newIds = new Set(checkedTaskIds);
						if (checked) {
							newIds.add(taskId);
						} else {
							newIds.delete(taskId);
						}
						onCheckChange(newIds);
					}}
					canCheck={canCheckTasks}
				/>
			);
		});
	}

	return (
		<Table>
			<TableHeader>
				<TableRow>
					<TableHead className="w-1/3">
						<div className="flex items-center gap-5">
							{canCheckTasks && (
								<Checkbox
									disabled={!tasks || tasks.length === 0}
									checked={
										tasks &&
										tasks.length > 0 &&
										checkedTaskIds.size === tasks.length
									}
									onCheckedChange={(checked) => {
										if (!tasks || !onCheckChange) {
											return;
										}

										if (!checked) {
											onCheckChange(new Set());
										} else {
											onCheckChange(new Set(tasks.map((t) => t.id)));
										}
									}}
									aria-label="Select all tasks"
								/>
							)}
							Task
						</div>
					</TableHead>
					<TableHead>Status</TableHead>
					<TableHead>Created by</TableHead>
					<TableHead />
				</TableRow>
			</TableHeader>
			<TableBody className="[&_td]:h-[72px]">{body}</TableBody>
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
			<TableCell colSpan={4} className="text-center">
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

type TaskRowProps = {
	task: Task;
	checked: boolean;
	onCheckChange: (taskId: string, checked: boolean) => void;
	canCheck: boolean;
};

const TaskRow: FC<TaskRowProps> = ({
	task,
	checked,
	onCheckChange,
	canCheck,
}) => {
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
	const templateDisplayName = task.template_display_name ?? task.template_name;
	const navigate = useNavigate();
	const queryClient = useQueryClient();

	const showPause =
		task.status === "active" ||
		task.status === "initializing" ||
		task.status === "pending";
	const pauseDisabled =
		task.status === "pending" || task.status === "initializing";
	const showResume = task.status === "paused" || task.status === "error";

	const pauseMutation = useMutation({
		mutationFn: async () => {
			if (!task.workspace_id) {
				throw new Error("Workspace ID is not available");
			}
			return API.stopWorkspace(task.workspace_id);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
		onError: (error: unknown) => {
			displayError(getErrorMessage(error, "Failed to pause task."));
		},
	});

	const resumeMutation = useMutation({
		mutationFn: async () => {
			if (!task.workspace_id) {
				throw new Error("Workspace ID is not available");
			}
			return API.startWorkspace(
				task.workspace_id,
				task.template_version_id,
				undefined,
				undefined,
			);
		},
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["tasks"] });
		},
		onError: (error: unknown) => {
			displayError(getErrorMessage(error, "Failed to resume task."));
		},
	});

	const taskPageLink = `/tasks/${task.owner_name}/${task.id}`;
	// Discard role, breaks Chromatic.
	const { role, ...clickableRowProps } = useClickableTableRow({
		onClick: () => navigate(taskPageLink),
	});

	return (
		<>
			<TableRow
				key={task.id}
				data-testid={`task-${task.id}`}
				{...clickableRowProps}
			>
				<TableCell>
					<div className="flex items-center gap-5">
						{canCheck && (
							<Checkbox
								data-testid={`checkbox-${task.id}`}
								checked={checked}
								onClick={(e) => {
									e.stopPropagation();
								}}
								onCheckedChange={(checked) => {
									onCheckChange(task.id, Boolean(checked));
								}}
								aria-label={`Select task ${task.initial_prompt}`}
							/>
						)}
						<AvatarData
							title={
								<span className="block max-w-[520px] truncate">
									{task.display_name}
								</span>
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
					</div>
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
					<div className="flex items-center justify-end gap-1">
						{showPause && (
							<TaskActionButton
								action="pause"
								disabled={pauseDisabled}
								loading={pauseMutation.isPending}
								onClick={() => pauseMutation.mutate()}
							/>
						)}
						{showResume && (
							<TaskActionButton
								action="resume"
								loading={resumeMutation.isPending}
								onClick={() => resumeMutation.mutate()}
							/>
						)}
						<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<Button
									size="icon-lg"
									variant="subtle"
									onClick={(e) => e.stopPropagation()}
								>
									<EllipsisVertical aria-hidden="true" />
									<span className="sr-only">Show task actions</span>
								</Button>
							</DropdownMenuTrigger>
							<DropdownMenuContent align="end">
								<DropdownMenuItem
									className="text-content-destructive focus:text-content-destructive"
									onClick={(e) => {
										e.stopPropagation();
										setIsDeleteDialogOpen(true);
									}}
								>
									<TrashIcon />
									Delete&hellip;
								</DropdownMenuItem>
							</DropdownMenuContent>
						</DropdownMenu>
					</div>
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

type TasksSkeletonProps = {
	canCheckTasks: boolean;
};

const TasksSkeleton: FC<TasksSkeletonProps> = ({ canCheckTasks }) => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell>
					<div className="flex items-center gap-5">
						{canCheckTasks && <Checkbox disabled />}
						<AvatarDataSkeleton />
					</div>
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
