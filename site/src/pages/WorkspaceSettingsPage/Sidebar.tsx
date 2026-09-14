import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import { useWorkspaceSettings } from "./useWorkspaceSettings";

export const Sidebar: React.FC = () => {
	const { permissions } = useWorkspaceSettings();

	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				<SettingsSidebarNavItem end href="">
					General
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="parameters">
					Parameters
				</SettingsSidebarNavItem>
				<SettingsSidebarNavItem href="schedule">
					Schedule
				</SettingsSidebarNavItem>
				{permissions?.shareWorkspace && (
					<SettingsSidebarNavItem href="sharing">
						Sharing
					</SettingsSidebarNavItem>
				)}
			</div>
		</BaseSidebar>
	);
};
