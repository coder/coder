import { API } from "api/api";
import { cva } from "class-variance-authority";
import { Button } from "components/Button/Button";
import { CoderIcon } from "components/Icons/CoderIcon";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import {
	ArrowLeftIcon,
	EditIcon,
	PanelLeftCloseIcon,
	PanelLeftOpenIcon,
} from "lucide-react";
import type { Task } from "modules/tasks/tasks";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router";
import { cn } from "utils/cn";
import { UsersCombobox } from "./UsersCombobox";

export const TasksSidebar: FC = () => {
	const { user, permissions } = useAuthenticated();
	const usernameParam = useSearchParamsKey({
		key: "username",
		defaultValue: user.username,
	});

	const [isCollapsed, setIsCollapsed] = useState(false);

	return (
		<div
			className={cn(
				"flex flex-col flex-1 min-h-0 gap-6 bg-surface-secondary max-w-80 border-solid border-0 border-r transition-all pt-3",
				isCollapsed && "max-w-16 items-center",
			)}
		>
			<div className="px-3 flex flex-col gap-6">
				{isCollapsed ? (
					<Button
						variant="subtle"
						size="icon"
						onClick={() => setIsCollapsed(false)}
					>
						<PanelLeftOpenIcon />
					</Button>
				) : (
					<div className="flex items-center place-content-between">
						<Button variant="outline" size="sm" asChild={true}>
							<RouterLink to="/tasks">
								<ArrowLeftIcon /> Tasks
							</RouterLink>
						</Button>
						<Button
							size="icon"
							variant="outline"
							onClick={() => setIsCollapsed(true)}
						>
							<PanelLeftCloseIcon />
						</Button>
					</div>
				)}

				<Button
					variant={isCollapsed ? "subtle" : "default"}
					size={isCollapsed ? "icon" : "sm"}
					asChild={true}
				>
					<RouterLink to="/tasks">
						<span className={isCollapsed ? "hidden" : ""}>New Task</span>{" "}
						<EditIcon />
					</RouterLink>
				</Button>
			</div>

			<div
				className={cn(
					"flex flex-col flex-1 min-h-0 gap-4",
					isCollapsed && "hidden",
				)}
			>
				{permissions.viewAllUsers && (
					<div className="flex w-full px-3">
						<UsersCombobox
							value={usernameParam.value}
							onValueChange={(username) => {
								if (username === usernameParam.value) {
									usernameParam.setValue("");
									return;
								}
								usernameParam.setValue(username);
							}}
						/>
					</div>
				)}
				<TasksSidebarGroup username={usernameParam.value} />
			</div>
		</div>
	);
};

type TasksSidebarGroupProps = {
	username: string;
};

const TasksSidebarGroup: FC<TasksSidebarGroupProps> = ({ username }) => {
	const filter = { username };
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.experimental.getTasks(filter),
		refetchInterval: 10_000,
	});

	return (
		<div className="flex flex-col flex-1 gap-2 min-h-0 transition-[opacity] group-data-[collapsible=icon]:opacity-0">
			<div className="text-content-secondary text-xs px-3">Tasks</div>
			<div className="flex flex-col flex-1 gap-1 min-h-0 overflow-y-auto">
				{tasksQuery.data ? (
					tasksQuery.data.map((t) => (
						<TaskSidebarMenuItem key={t.workspace.id} task={t} />
					))
				) : (
					<div className="flex flex-col gap-1 px-3">
						{Array.from({ length: 5 }).map((_, index) => (
							<div
								key={index}
								aria-hidden={true}
								className="h-8 w-full rounded-lg bg-surface-tertiary animate-pulse"
							/>
						))}
					</div>
				)}
			</div>
		</div>
	);
};

type TaskSidebarMenuItemProps = {
	task: Task;
};

const TaskSidebarMenuItem: FC<TaskSidebarMenuItemProps> = ({ task }) => {
	const { workspace } = useParams<{ workspace: string }>();

	return (
		<div className="px-3">
			<Button
				size="sm"
				variant="subtle"
				className={cn(
					"w-full justify-start",
					task.workspace.name !== workspace
						? "text-content-secondary"
						: "text-content-primary bg-surface-tertiary",
				)}
				asChild
			>
				<RouterLink
					to={{
						pathname: `/tasks/${task.workspace.owner_name}/${task.workspace.name}`,
						search: window.location.search,
					}}
				>
					<TaskSidebarMenuItemStatus task={task} />
					<span>{task.workspace.name}</span>
				</RouterLink>
			</Button>
		</div>
	);
};

const taskStatusVariants = cva("block size-2 rounded-full shrink-0", {
	variants: {
		state: {
			default: "border border-content-secondary border-solid",
			complete: "bg-content-success",
			failure: "bg-content-destructive",
			idle: "bg-content-secondary",
			working: "bg-highlight-sky",
		},
	},
	defaultVariants: {
		state: "default",
	},
});

const TaskSidebarMenuItemStatus: FC<{ task: Task }> = ({ task }) => {
	const statusText = task.workspace.latest_app_status
		? task.workspace.latest_app_status.state
		: "No activity yet";

	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<div
						className={taskStatusVariants({
							state: task.workspace.latest_app_status?.state ?? "default",
						})}
					>
						<span className="sr-only">{statusText}</span>
					</div>
				</TooltipTrigger>
				<TooltipContent className="first-letter:capitalize">
					{statusText}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
