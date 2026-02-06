import {
	type CellContext,
	type ColumnDef,
	flexRender,
	getCoreRowModel,
	type HeaderContext,
	type Table as ReactTable,
	type Row,
	useReactTable,
} from "@tanstack/react-table";
import { API } from "api/api";
import { getErrorDetail, getErrorMessage } from "api/errors";
import type { Task, TaskStatus as TaskStatusType } from "api/typesGenerated";
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
import { useClickableTableRow, useRowRangeSelection } from "hooks";
import { EllipsisVertical, RotateCcwIcon, TrashIcon } from "lucide-react";
import { TaskDeleteDialog } from "modules/tasks/TaskDeleteDialog/TaskDeleteDialog";
import { TaskStatus } from "modules/tasks/TaskStatus/TaskStatus";
import { type FC, Fragment, type ReactNode, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { relativeTime } from "utils/time";
import { TaskActionButton } from "./TaskActionButton";

interface TasksTableMeta {
	checkedTaskIds: Set<string>;
	canCheckTasks: boolean;
	onCheckChange: (taskIds: Set<string>) => void;
	onRowShiftClick: (
		event: React.MouseEvent<HTMLButtonElement>,
		row: Row<Task>,
		table: ReactTable<Task>,
	) => Array<Row<Task>> | null;
}

function NameHeader({ table }: HeaderContext<Task, unknown>): React.ReactNode {
	const { checkedTaskIds, canCheckTasks, onCheckChange } = table.options
		.meta as TasksTableMeta;
	const tasks = table.options.data;
	const allChecked = tasks.length > 0 && checkedTaskIds.size === tasks.length;
	const someChecked = !allChecked && checkedTaskIds.size > 0;

	return (
		<TableHead className="w-1/3">
			<div className="flex items-center gap-5">
				{canCheckTasks && (
					<Checkbox
						data-testid="checkbox-all"
						disabled={tasks.length === 0}
						checked={allChecked ? true : someChecked ? "indeterminate" : false}
						onClick={(e) => {
							e.stopPropagation();
						}}
						onCheckedChange={(checked) => {
							if (checked) {
								onCheckChange(new Set(tasks.map((t) => t.id)));
							} else {
								onCheckChange(new Set());
							}
						}}
						aria-label="Select all tasks"
					/>
				)}
				<span>Task</span>
			</div>
		</TableHead>
	);
}

function NameCell({ row, table }: CellContext<Task, unknown>): React.ReactNode {
	const { checkedTaskIds, canCheckTasks, onCheckChange, onRowShiftClick } =
		table.options.meta as TasksTableMeta;
	const task = row.original;
	const checked = checkedTaskIds.has(task.id);
	const templateDisplayName = task.template_display_name ?? task.template_name;

	return (
		<TableCell>
			<div className="flex items-center gap-5">
				{canCheckTasks && (
					<Checkbox
						data-testid={`checkbox-${task.id}`}
						checked={checked}
						onClick={(e) => {
							e.stopPropagation();
							const rowsToToggle = onRowShiftClick(e, row, table);
							if (rowsToToggle) {
								// Shift+click: toggle all rows in range
								const newIds = new Set(checkedTaskIds);
								const shouldSelect = !checked;
								for (const r of rowsToToggle) {
									if (shouldSelect) {
										newIds.add(r.original.id);
									} else {
										newIds.delete(r.original.id);
									}
								}
								onCheckChange(newIds);
								e.preventDefault(); // Prevent onCheckedChange from firing
							}
						}}
						onCheckedChange={(checked) => {
							const newIds = new Set(checkedTaskIds);
							if (checked) {
								newIds.add(task.id);
							} else {
								newIds.delete(task.id);
							}
							onCheckChange(newIds);
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
	);
}

function StatusCell({ row }: CellContext<Task, unknown>): React.ReactNode {
	const task = row.original;

	return (
		<TableCell>
			<TaskStatus
				status={task.status}
				stateMessage={task.current_state?.message || "No message available"}
			/>
		</TableCell>
	);
}

function CreatedByCell({ row }: CellContext<Task, unknown>): React.ReactNode {
	const task = row.original;

	return (
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
	);
}

const pauseStatuses: TaskStatusType[] = [
	"active",
	"initializing",
	"pending",
	"error",
	"unknown",
];
const pauseDisabledStatuses: TaskStatusType[] = ["pending", "initializing"];
const resumeStatuses: TaskStatusType[] = ["paused", "error", "unknown"];

function ActionsCell({ row }: CellContext<Task, unknown>): React.ReactNode {
	const task = row.original;
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
	const queryClient = useQueryClient();

	const showPause = pauseStatuses.includes(task.status);
	const pauseDisabled = pauseDisabledStatuses.includes(task.status);
	const showResume = resumeStatuses.includes(task.status);

	const pauseMutation = useMutation({
		mutationFn: async () => {
			if (!task.workspace_id) {
				throw new Error("Task has no workspace");
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
				throw new Error("Task has no workspace");
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

	return (
		<TableCell className="text-right">
			<div className="flex items-center justify-end gap-1">
				{showPause && (
					<TaskActionButton
						action="pause"
						disabled={pauseDisabled}
						loading={pauseMutation.isPending}
						onClick={pauseMutation.mutate}
					/>
				)}
				{showResume && (
					<TaskActionButton
						action="resume"
						loading={resumeMutation.isPending}
						onClick={resumeMutation.mutate}
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

			<TaskDeleteDialog
				task={task}
				open={isDeleteDialogOpen}
				onClose={() => {
					setIsDeleteDialogOpen(false);
				}}
			/>
		</TableCell>
	);
}

const columns: ColumnDef<Task, unknown>[] = [
	{
		id: "name",
		header: NameHeader,
		cell: NameCell,
	},
	{
		id: "status",
		header: () => <TableHead>Status</TableHead>,
		cell: StatusCell,
	},
	{
		id: "createdBy",
		header: () => <TableHead>Created by</TableHead>,
		cell: CreatedByCell,
	},
	{
		id: "actions",
		header: () => (
			<TableHead className="w-0">
				<span className="sr-only">Actions</span>
			</TableHead>
		),
		cell: ActionsCell,
	},
];

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
	const isLoading = !tasks && !error;
	const { handleShiftClick: onRowShiftClick } = useRowRangeSelection<Task>();

	const table = useReactTable({
		data: (tasks ?? []) as Task[],
		columns,
		getCoreRowModel: getCoreRowModel(),
		meta: {
			checkedTaskIds,
			canCheckTasks,
			onCheckChange: onCheckChange ?? (() => {}),
			onRowShiftClick,
		} satisfies TasksTableMeta,
	});

	return (
		<Table>
			<TableHeader>
				{table.getHeaderGroups().map((headerGroup) => (
					<TableRow key={headerGroup.id}>
						{headerGroup.headers.map((header) => (
							<Fragment key={header.id}>
								{header.isPlaceholder
									? null
									: flexRender(
											header.column.columnDef.header,
											header.getContext(),
										)}
							</Fragment>
						))}
					</TableRow>
				))}
			</TableHeader>
			<TableBody className="[&_td]:h-[72px]">
				{error ? (
					<TasksErrorBody error={error} onRetry={onRetry} />
				) : isLoading ? (
					<TasksLoader canCheckTasks={canCheckTasks} />
				) : table.getRowModel().rows.length > 0 ? (
					table.getRowModel().rows.map((row) => (
						<TaskRow
							key={row.id}
							task={row.original}
							checked={checkedTaskIds.has(row.original.id)}
						>
							{row
								.getVisibleCells()
								.map((cell) =>
									flexRender(cell.column.columnDef.cell, cell.getContext()),
								)}
						</TaskRow>
					))
				) : (
					<TasksEmpty />
				)}
			</TableBody>
		</Table>
	);
};

interface TaskRowProps {
	task: Task;
	children?: ReactNode;
	checked: boolean;
}

const TaskRow: FC<TaskRowProps> = ({ task, children, checked }) => {
	const navigate = useNavigate();
	const taskPageLink = `/tasks/${task.owner_name}/${task.id}`;

	// Discard role, breaks Chromatic.
	const { role, ...clickableRowProps } = useClickableTableRow({
		onClick: () => navigate(taskPageLink),
	});

	return (
		<TableRow
			data-testid={`task-${task.id}`}
			data-state={checked ? "selected" : undefined}
			{...clickableRowProps}
		>
			{children}
		</TableRow>
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

interface TasksLoaderProps {
	canCheckTasks: boolean;
}

const TasksLoader: FC<TasksLoaderProps> = ({ canCheckTasks }) => {
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
