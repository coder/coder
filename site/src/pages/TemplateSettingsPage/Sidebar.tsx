import type { Template } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import {
	Sidebar as BaseSidebar,
	SidebarHeader,
	SidebarNavItem,
} from "components/Sidebar/Sidebar";
import {
	LockIcon,
	TimerIcon as ScheduleIcon,
	SettingsIcon,
	CodeIcon as VariablesIcon,
} from "lucide-react";
import { linkToTemplate, useLinks } from "modules/navigation";
import type { FC } from "react";

interface SidebarProps {
	template: Template;
}

export const Sidebar: FC<SidebarProps> = ({ template }) => {
	const getLink = useLinks();

	return (
		<BaseSidebar>
			<SidebarHeader
				avatar={
					<Avatar variant="icon" src={template.icon} fallback={template.name} />
				}
				title={template.display_name || template.name}
				linkTo={getLink(
					linkToTemplate(template.organization_name, template.name),
				)}
				subtitle={template.name}
			/>

			<SidebarNavItem href="" icon={SettingsIcon}>
				General
			</SidebarNavItem>
			<SidebarNavItem href="permissions" icon={LockIcon}>
				Permissions
			</SidebarNavItem>
			<SidebarNavItem href="variables" icon={VariablesIcon}>
				Variables
			</SidebarNavItem>
			<SidebarNavItem href="schedule" icon={ScheduleIcon}>
				Schedule
			</SidebarNavItem>
		</BaseSidebar>
	);
};
