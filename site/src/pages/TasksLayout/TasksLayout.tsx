import { SidebarProvider } from "components/LayotuSidebar/LayoutSidebar";
import type { FC } from "react";
import { Outlet } from "react-router";
import { TasksSidebar } from "./TasksSidebar";

export const TasksLayout: FC = () => {
	return (
		<SidebarProvider>
			<TasksSidebar />
			<main className="flex-1">
				<Outlet />
			</main>
		</SidebarProvider>
	);
};

export default TasksLayout;
