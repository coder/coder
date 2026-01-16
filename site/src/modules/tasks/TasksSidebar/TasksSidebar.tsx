import { API } from "api/api";
import { getErrorMessage } from "api/errors";
import type { Task, TasksFilter } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuGroup,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { CoderIcon } from "components/Icons/CoderIcon";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Skeleton } from "components/Skeleton/Skeleton";
import { StatusIndicatorDot } from "components/StatusIndicator/StatusIndicator";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import {
	EditIcon,
	EllipsisIcon,
	PanelLeftIcon,
	Share2Icon,
	TrashIcon,
} from "lucide-react";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useNavigate, useParams } from "react-router";
import { cn } from "utils/cn";
import { TaskDeleteDialog } from "../TaskDeleteDialog/TaskDeleteDialog";
import { taskStatusToStatusIndicatorVariant } from "../TaskStatus/TaskStatus";
import { UserCombobox } from "./UserCombobox";

export const TasksSidebar: FC = () => {
	const { user, permissions } = useAuthenticated();
	const ownerParam = useSearchParamsKey({
		key: "owner",
		defaultValue: user.username,
	});

	const [isCollapsed, setIsCollapsed] = useState(false);

	return (
		<div
			className={cn(
				"h-full bg-surface-secondary w-full max-w-80",
				"border-solid border-0 border-r transition-all flex flex-col",
				{ "max-w-14": isCollapsed },
			)}
		>
			<div className="p-3 flex flex-col gap-6">
				<div className="flex items-center place-content-between">
					{!isCollapsed && (
						<Button
							size="icon"
							variant="subtle"
							className={cn(["size-8 p-0 transition-[margin,opacity]"])}
							asChild
						>
							<RouterLink to="/tasks">
								<CoderIcon className="fill-content-primary !size-6 !p-0" />
								<span className="sr-only">Navigate to tasks</span>
							</RouterLink>
						</Button>
					)}

					<TooltipProvider>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									size="icon"
									variant="subtle"
									onClick={() => setIsCollapsed((v) => !v)}
									className="[&_svg]:p-0"
								>
									<PanelLeftIcon />
									<span className="sr-only">
										{isCollapsed ? "Open" : "Close"} Sidebar
									</span>
								</Button>
							</TooltipTrigger>
							<TooltipContent side="right" align="center">
								{isCollapsed ? "Open" : "Close"} Sidebar
							</TooltipContent>
						</Tooltip>
					</TooltipProvider>
				</div>

				<TooltipProvider>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant={isCollapsed ? "subtle" : "default"}
								size={isCollapsed ? "icon" : "sm"}
								asChild={true}
								className={cn({
									"[&_svg]:p-0": isCollapsed,
								})}
							>
								<RouterLink to="/tasks">
									<span className={isCollapsed ? "hidden" : ""}>New Task</span>{" "}
									<EditIcon />
								</RouterLink>
							</Button>
						</TooltipTrigger>
						<TooltipContent side="right" align="center">
							New task
						</TooltipContent>
					</Tooltip>
				</TooltipProvider>

				{!isCollapsed && permissions.viewAllUsers && (
					<UserCombobox
						value={ownerParam.value}
						onValueChange={(username) => {
							if (username === ownerParam.value) {
								ownerParam.setValue("");
								return;
							}
							ownerParam.setValue(username);
						}}
					/>
				)}
			</div>

			{!isCollapsed && <TasksSidebarGroup owner={ownerParam.value} />}
		</div>
	);
};

type TasksSidebarGroupProps = {
	owner: string;
};

const TasksSidebarGroup: FC<TasksSidebarGroupProps> = ({ owner }) => {
	const filter: TasksFilter = { owner };
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.getTasks(filter),
		refetchInterval: 10_000,
	});

	return (
		<ScrollArea className="flex-1">
			<div className="flex flex-col gap-2 p-3">
				<div className="text-content-secondary text-xs">Tasks</div>
				<div className="flex flex-col flex-1 gap-1">
					{tasksQuery.data ? (
						tasksQuery.data.length > 0 ? (
							tasksQuery.data.map((task) => (
								<TaskSidebarMenuItem key={task.id} task={task} />
							))
						) : (
							<div className="text-content-secondary text-xs p-4 border-border border-solid rounded text-center">
								No tasks found
							</div>
						)
					) : tasksQuery.error ? (
						<div className="text-content-secondary text-xs p-4 border-border border-solid rounded text-center">
							{getErrorMessage(tasksQuery.error, "Failed to load tasks")}
						</div>
					) : (
						<div className="flex flex-col gap-1">
							{Array.from({ length: 5 }).map((_, index) => (
								<Skeleton className="h-8 w-full" key={index} />
							))}
						</div>
					)}
				</div>
			</div>
		</ScrollArea>
	);
};

type TaskSidebarMenuItemProps = {
	task: Task;
};

const TaskSidebarMenuItem: FC<TaskSidebarMenuItemProps> = ({ task }) => {
	const { taskId } = useParams<{ taskId: string }>();
	const isActive = task.id === taskId;
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false);
	const navigate = useNavigate();

	return (
		<>
			<Button
				asChild
				size="sm"
				variant="subtle"
				className={cn(
					"overflow-visible group w-full justify-start text-content-secondary",
					"transition-none hover:bg-surface-tertiary gap-2 has-[[data-state=open]]:bg-surface-tertiary",
					{
						"text-content-primary bg-surface-quaternary hover:bg-surface-quaternary has-[[data-state=open]]:bg-surface-quaternary":
							isActive,
					},
				)}
			>
				<RouterLink
					to={{
						pathname: `/tasks/${task.owner_name}/${task.id}`,
						search: window.location.search,
					}}
				>
					<TaskSidebarMenuItemStatus task={task} />
					<span className="block max-w-[220px] truncate">
						{task.display_name}
					</span>
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className={`
								ml-auto -mr-2 opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100
								focus-visible:opacity-100 data-[state=open]:opacity-100
							`}
								onClick={(e) => {
									e.stopPropagation();
									e.preventDefault();
								}}
							>
								<EllipsisIcon />
								<span className="sr-only">Task options</span>
							</Button>
						</DropdownMenuTrigger>

						<DropdownMenuContent align="end">
							<DropdownMenuGroup>
								<DropdownMenuItem asChild>
									<RouterLink
										to={`/@${task.owner_name}/${task.workspace_name}/settings/sharing`}
									>
										<Share2Icon />
										Share workspace
									</RouterLink>
								</DropdownMenuItem>
								<DropdownMenuSeparator />
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
							</DropdownMenuGroup>
						</DropdownMenuContent>
					</DropdownMenu>
				</RouterLink>
			</Button>

			<TaskDeleteDialog
				open={isDeleteDialogOpen}
				task={task}
				onClose={() => {
					setIsDeleteDialogOpen(false);
				}}
				onSuccess={() => {
					if (isActive) {
						navigate("/tasks");
					}
				}}
			/>
		</>
	);
};

const TaskSidebarMenuItemStatus: FC<{ task: Task }> = ({ task }) => {
	return (
		<TooltipProvider>
			<Tooltip>
				<TooltipTrigger asChild>
					<StatusIndicatorDot
						variant={taskStatusToStatusIndicatorVariant[task.status]}
						aria-label={task.status}
					/>
				</TooltipTrigger>
				<TooltipContent className="first-letter:capitalize">
					{task.status}
				</TooltipContent>
			</Tooltip>
		</TooltipProvider>
	);
};
