import { BotIcon, ShieldCheckIcon, StoreIcon } from "lucide-react";
import type { FC } from "react";
import { Sidebar, SidebarGroup, SidebarNavItem } from "#/components/Sidebar";
import type { Permissions } from "#/modules/permissions";

interface AISettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
}

/**
 * Displays navigation for the AI settings section. AI Governance and
 * Coder Agents group related pages; their headers link to the section
 * overview while child items own the active state. Providers is a flat
 * link.
 */
const AISettingsSidebarView: FC<AISettingsSidebarViewProps> = ({
	permissions,
}) => {
	const canViewGovernance =
		permissions.viewDeploymentConfig || permissions.viewAIGatewayKeys;

	return (
		<Sidebar>
			<div className="flex flex-col gap-1">
				{permissions.viewAnyAIProvider && (
					<SidebarNavItem icon={StoreIcon} href="/ai/settings/providers">
						Providers
					</SidebarNavItem>
				)}
				{canViewGovernance && (
					<SidebarGroup
						icon={ShieldCheckIcon}
						label="AI Governance"
						href={
							permissions.viewDeploymentConfig
								? "/ai/settings/governance"
								: "/ai/settings/gateway-keys"
						}
					>
						{permissions.viewDeploymentConfig && (
							<SidebarNavItem href="/ai/settings/governance">
								Overview
							</SidebarNavItem>
						)}
						{permissions.viewAIGatewayKeys && (
							<SidebarNavItem href="/ai/settings/gateway-keys">
								AI Gateway keys
							</SidebarNavItem>
						)}
					</SidebarGroup>
				)}
				{permissions.editDeploymentConfig && (
					<SidebarGroup
						icon={BotIcon}
						label="Coder Agents"
						href="/ai/settings/coder-agents"
					>
						<SidebarNavItem end href="/ai/settings/coder-agents">
							Overview
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/models">Models</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/mcp-servers">
							MCP servers
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/templates">
							Templates
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/spend">Spend</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/instructions">
							Instructions
						</SidebarNavItem>
						<SidebarNavItem href="/ai/settings/lifecycle">
							Lifecycle
						</SidebarNavItem>
					</SidebarGroup>
				)}
			</div>
		</Sidebar>
	);
};

export default AISettingsSidebarView;
