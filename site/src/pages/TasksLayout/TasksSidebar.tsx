import { API } from "api/api";
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
import { useAuthenticated } from "hooks";
import { useSearchParamsKey } from "hooks/useSearchParamsKey";
import { SquarePenIcon } from "lucide-react";
import { UsersCombobox } from "pages/TasksPage/UsersCombobox";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router";
import { cn } from "utils/cn";

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
	const { workspace } = useParams<{ workspace: string }>();
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
								<SidebarMenuItem key={t.workspace.id}>
									<SidebarMenuButton
										isActive={t.workspace.name === workspace}
										asChild
									>
										<RouterLink
											to={{
												pathname: `/tasks/${t.workspace.owner_name}/${t.workspace.name}`,
												search: window.location.search,
											}}
										>
											<span>{t.workspace.name}</span>
										</RouterLink>
									</SidebarMenuButton>
								</SidebarMenuItem>
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
