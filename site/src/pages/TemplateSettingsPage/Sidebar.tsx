import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
} from "#/components/Sidebar/Sidebar";

export const Sidebar: React.FC<{ canUpdateTemplate: boolean }> = ({
	canUpdateTemplate,
}) => {
	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				{canUpdateTemplate && (
					<>
						<SettingsSidebarNavItem end href="">
							General
						</SettingsSidebarNavItem>
						<SettingsSidebarNavItem href="permissions">
							Permissions
						</SettingsSidebarNavItem>
						<SettingsSidebarNavItem href="variables">
							Variables
						</SettingsSidebarNavItem>
					</>
				)}
				<SettingsSidebarNavItem href="parameters">
					Parameters
				</SettingsSidebarNavItem>
				{canUpdateTemplate && (
					<SettingsSidebarNavItem href="schedule">
						Schedule
					</SettingsSidebarNavItem>
				)}
			</div>
		</BaseSidebar>
	);
};
