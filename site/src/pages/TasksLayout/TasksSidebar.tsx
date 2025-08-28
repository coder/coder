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
	useSidebar,
} from "components/LayotuSidebar/LayoutSidebar";
import { useAuthenticated } from "hooks";
import { SquarePenIcon } from "lucide-react";
import type { FC } from "react";
import { useQuery } from "react-query";
import { Link as RouterLink, useParams } from "react-router";
import { cn } from "utils/cn";

export const TasksSidebar: FC = () => {
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

				<TasksSidebarGroup />
			</SidebarContent>
		</Sidebar>
	);
};

const TasksSidebarHeader: FC = () => {
	const sidebar = useSidebar();

	return (
		<SidebarHeader>
			<div className="flex items-center ">
				<Button
					size="icon"
					variant="subtle"
					className={cn([
						"size-8 p-0 transition-[margin,opacity] ml-1",
						{ "-ml-10 opacity-0": !sidebar.open },
					])}
				>
					<CoderIcon className="fill-content-primary !size-6 !p-0" />
				</Button>
				<SidebarTrigger className="ml-auto" />
			</div>
		</SidebarHeader>
	);
};

const TasksSidebarGroup: FC = () => {
	const sidebar = useSidebar();
	const { user } = useAuthenticated();
	const { workspace } = useParams<{ workspace: string }>();
	const filter = { username: user.username };
	const tasksQuery = useQuery({
		queryKey: ["tasks", filter],
		queryFn: () => API.experimental.getTasks(filter),
		refetchInterval: 10_000,
	});

	return (
		<SidebarGroup
			className={cn([
				"transition-[opacity] opacity-0",
				{ "opacity-100": sidebar.open },
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
											to={`/tasks/${t.workspace.owner_name}/${t.workspace.name}`}
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
