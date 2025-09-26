import type { FC, PropsWithChildren } from "react";
import { Outlet } from "react-router";
import { TasksSidebar } from "../TasksSidebar/TasksSidebar";

const TasksLayout: FC<PropsWithChildren> = () => {
	return (
		<div className="flex items-stretch h-full">
			<TasksSidebar />
			<div className="flex flex-col h-full flex-1 overflow-y-auto">
				<Outlet />
			</div>
		</div>
	);
};

export default TasksLayout;
