import { Sidebar as BaseSidebar, SidebarNavItem } from "#/components/Sidebar";

export const Sidebar: React.FC = () => {
	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SidebarNavItem end href="">
					General
				</SidebarNavItem>
				<SidebarNavItem href="permissions">Permissions</SidebarNavItem>
				<SidebarNavItem href="variables">Variables</SidebarNavItem>
				<SidebarNavItem href="schedule">Schedule</SidebarNavItem>
			</div>
		</BaseSidebar>
	);
};
