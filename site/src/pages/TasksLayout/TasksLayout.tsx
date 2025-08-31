import { SidebarProvider } from "components/LayoutSidebar/LayoutSidebar";
import type { FC } from "react";
import { Outlet } from "react-router";
import { TasksSidebar } from "./TasksSidebar";

const TasksLayout: FC = () => {
	return (
		<SidebarProvider className="flex flex-1 min-h-0 h-full">
			<TasksSidebar />
			<main className="flex-1">
				<Outlet />
			</main>
		</SidebarProvider>
	);
};

export default TasksLayout;
