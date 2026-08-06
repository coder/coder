import { Sidebar as BaseSidebar, SidebarNavItem } from "#/components/Sidebar";
import { useWorkspaceSettings } from "./useWorkspaceSettings";

export const Sidebar: React.FC = () => {
	const { permissions } = useWorkspaceSettings();

	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SidebarNavItem end href="">
					General
				</SidebarNavItem>
				<SidebarNavItem href="parameters">Parameters</SidebarNavItem>
				<SidebarNavItem href="schedule">Schedule</SidebarNavItem>
				{permissions?.shareWorkspace && (
					<SidebarNavItem href="sharing">Sharing</SidebarNavItem>
				)}
			</div>
		</BaseSidebar>
	);
};
