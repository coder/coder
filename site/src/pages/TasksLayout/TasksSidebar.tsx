import { API } from "api/api";
import { cva } from "class-variance-authority";
import { Button } from "components/Button/Button";
import { CoderIcon } from "components/Icons/CoderIcon";
import {
	Sidebar,
	SidebarContent,
	SidebarGroup,
	SidebarGroupContent,
	SidebarGroupLabel,
	SidebarHeader,
	SidebarMenu,
	SidebarMenuButton,
	SidebarMenuItem,
	SidebarMenuSkeleton,
	SidebarTrigger,
} from "components/LayotuSidebar/LayoutSidebar";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { SquarePenIcon } from "lucide-react";
import type { Task } from "modules/tasks/tasks";
import type { FC } from "react";
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

	return (
		<Sidebar collapsible="icon">
			<TasksSidebarHeader />

			<SidebarContent>
				<SidebarGroup>
					<SidebarGroupContent>
						<SidebarMenu>
							<SidebarMenuItem>
								<SidebarMenuButton asChild>
									<RouterLink to="/tasks">
										<SquarePenIcon />
										<span>New task</span>
									</RouterLink>
								</SidebarMenuButton>
							</SidebarMenuItem>
						</SidebarMenu>
					</SidebarGroupContent>
				</SidebarGroup>

				{permissions.viewAllUsers && (
					<SidebarGroup className="transition-[margin,opacity] group-data-[collapsible=icon]:-mt-8 group-data-[collapsible=icon]:opacity-0">
						<SidebarGroupContent>
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
						</SidebarGroupContent>
					</SidebarGroup>
				)}

				<TasksSidebarGroup username={usernameParam.value} />
			</SidebarContent>
		</Sidebar>
	);
};

const TasksSidebarHeader: FC = () => {
	return (
		<SidebarHeader>
			<div className="flex items-center ">
				<Button
					size="icon"
					variant="subtle"
					className={cn([
						"size-8 p-0 transition-[margin,opacity] ml-1",
						"group-data-[collapsible=icon]:-ml-10 group-data-[collapsible=icon]:opacity-0",
					])}
				>
					<CoderIcon className="fill-content-primary !size-6 !p-0" />
				</Button>
				<SidebarTrigger className="ml-auto" />
			</div>
		</SidebarHeader>
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
		<SidebarGroup
			className={cn([
				"transition-[opacity] group-data-[collapsible=icon]:opacity-0",
			])}
		>
			<SidebarGroupLabel>Tasks</SidebarGroupLabel>
			<SidebarGroupContent>
				<SidebarMenu>
					{tasksQuery.data
						? tasksQuery.data.map((t) => (
								<TaskSidebarMenuItem key={t.workspace.id} task={t} />
							))
						: Array.from({ length: 5 }).map((_, index) => (
								<SidebarMenuItem key={index}>
									<SidebarMenuSkeleton />
								</SidebarMenuItem>
							))}
				</SidebarMenu>
			</SidebarGroupContent>
		</SidebarGroup>
	);
};

type TaskSidebarMenuItemProps = {
	task: Task;
};

export const TaskSidebarMenuItem: FC<TaskSidebarMenuItemProps> = ({ task }) => {
	const { workspace } = useParams<{ workspace: string }>();

	return (
		<SidebarMenuItem>
			<SidebarMenuButton isActive={task.workspace.name === workspace} asChild>
				<RouterLink
					to={{
						pathname: `/tasks/${task.workspace.owner_name}/${task.workspace.name}`,
						search: window.location.search,
					}}
				>
					<TaskSidebarMenuItemStatus task={task} />
					<span>{task.workspace.name}</span>
				</RouterLink>
			</SidebarMenuButton>
		</SidebarMenuItem>
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
