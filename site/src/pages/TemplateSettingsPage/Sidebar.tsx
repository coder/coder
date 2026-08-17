import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
} from "#/components/Sidebar/Sidebar";

export const Sidebar: React.FC = () => {
	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SettingsSidebarNavItem end href="">
					General
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="permissions">
					Permissions
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="variables">
					Variables
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="schedule">
					Schedule
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="ai-egress-policy">
					AI Egress Policy
				</SettingsSidebarNavItem>
			</div>
		</BaseSidebar>
	);
};
