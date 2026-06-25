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
				{permissions.viewAIGatewayKeys && (
					<SidebarNavItem href="/ai/settings/gateway-keys">
						AI Gateway Keys
					</SidebarNavItem>
				)}
				{permissions.viewAnyAIProvider && (
					<SidebarNavItem href="/ai/settings/providers">
						Providers
					</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/models">Models</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/instructions">
						Instructions
					</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/lifecycle">
						Lifecycle
					</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/templates">
						Templates
					</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/mcp-servers">
						MCP servers
					</SidebarNavItem>
				)}
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
