import { Avatar } from "#/components/Avatar/Avatar";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
	SidebarHeader,
} from "#/components/Sidebar/Sidebar";
import { useWorkspaceSettings } from "./useWorkspaceSettings";

export const Sidebar: React.FC = () => {
	const { owner, workspace, permissions } = useWorkspaceSettings();

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={
					<Avatar
						variant="icon"
						src={workspace.template_icon}
						fallback={workspace.name}
					/>
				}
				title={workspace.name}
				linkTo={`/@${owner}/${workspace.name}`}
				subtitle={workspace.template_display_name ?? workspace.template_name}
			/>
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
