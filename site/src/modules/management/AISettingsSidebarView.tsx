import { ArrowUpRightIcon } from "lucide-react";
import type { FC } from "react";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import type { Permissions } from "#/modules/permissions";

interface AISettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
}

const AISettingsSidebarView: FC<AISettingsSidebarViewProps> = ({
	permissions,
}) => {
	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				{permissions.viewDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/governance">
						AI Governance
					</SidebarNavItem>
				)}
				<SidebarNavItem href="/ai/settings">Providers</SidebarNavItem>
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/agents/settings/agents">
						<div className="flex flex-row items-center gap-1">
							Manage Coder Agents <ArrowUpRightIcon size={16} />
						</div>
					</SidebarNavItem>
				)}
			</div>
		</BaseSidebar>
	);
};

export default AISettingsSidebarView;
