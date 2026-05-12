import type { FC } from "react";
import type { Template } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem,
	SidebarHeader,
} from "#/components/Sidebar/Sidebar";
import { linkToTemplate, useLinks } from "#/modules/navigation";

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
			</div>
		</BaseSidebar>
	);
};
