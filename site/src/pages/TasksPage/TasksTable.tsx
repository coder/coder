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
import { RotateCcwIcon } from "lucide-react";
import type { Task } from "modules/tasks/tasks";
import { WorkspaceAppStatus } from "modules/workspaces/WorkspaceAppStatus/WorkspaceAppStatus";
import type { FC, ReactNode } from "react";
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
	} else if (tasks) {
		body = tasks.length === 0 ? <TasksEmpty /> : <Tasks tasks={tasks} />;
	} else {
		body = <TasksSkeleton />;
	}

	return (
		<Table className="mt-4">
			<TableHeader>
				<TableRow>
					<TableHead>Task</TableHead>
					<TableHead>Status</TableHead>
					<TableHead>Created by</TableHead>
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
			<TableCell colSpan={4} className="text-center">
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

type TasksProps = { tasks: Task[] };

const Tasks: FC<TasksProps> = ({ tasks }) => {
	return tasks.map(({ workspace, prompt }) => {
		const templateDisplayName =
			workspace.template_display_name ?? workspace.template_name;

		return (
			<TableRow key={workspace.id} className="relative" hover>
				<TableCell>
					<AvatarData
						title={
							<>
								<span className="block max-w-[520px] overflow-hidden text-ellipsis whitespace-nowrap">
									{prompt}
								</span>
								<RouterLink
									to={`/tasks/${workspace.owner_name}/${workspace.name}`}
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
								src={workspace.template_icon}
								fallback={templateDisplayName}
							/>
						}
					/>
				</TableCell>
				<TableCell>
					<WorkspaceAppStatus
						disabled={workspace.latest_build.status !== "running"}
						status={workspace.latest_app_status}
					/>
				</TableCell>
				<TableCell>
					<AvatarData
						title={workspace.owner_name}
						subtitle={
							<span className="block first-letter:uppercase">
								{relativeTime(new Date(workspace.created_at))}
							</span>
						}
						src={workspace.owner_avatar_url}
					/>
				</TableCell>
			</TableRow>
		);
	});
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
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};
