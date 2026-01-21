import type { Workspace } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	Sidebar as BaseSidebar,
	SidebarHeader,
	SidebarNavItem,
} from "components/Sidebar/Sidebar";
import {
	SettingsIcon as GeneralIcon,
	CodeIcon as ParameterIcon,
	TimerIcon as ScheduleIcon,
	Users as SharingIcon,
} from "lucide-react";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";

interface SidebarProps {
	username: string;
	workspace: Workspace;
	sharingDisabled?: boolean;
}

export const Sidebar: FC<SidebarProps> = ({
	username,
	workspace,
	sharingDisabled,
}) => {
	const { experiments } = useDashboard();

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
				linkTo={`/@${username}/${workspace.name}`}
				subtitle={workspace.template_display_name ?? workspace.template_name}
			/>

			<SidebarNavItem href="" icon={GeneralIcon}>
				General
			</SidebarNavItem>
			<SidebarNavItem href="parameters" icon={ParameterIcon}>
				Parameters
			</SidebarNavItem>
			<SidebarNavItem href="schedule" icon={ScheduleIcon}>
				Schedule
			</SidebarNavItem>
			{experiments.includes("workspace-sharing") && !sharingDisabled && (
				<SidebarNavItem href="sharing" icon={SharingIcon}>
					Sharing
				</SidebarNavItem>
			)}
		</BaseSidebar>
	);
};
