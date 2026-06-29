import type { FC, ReactNode } from "react";
import { NavLink } from "react-router";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import type { Permissions } from "#/modules/permissions";
import { cn } from "#/utils/cn";

interface AISettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
}

const SubNavItem: FC<{ href: string; children?: ReactNode }> = ({
	href,
	children,
}) => (
	<NavLink
		to={href}
		className={({ isActive }) =>
			cn(
				"relative -ml-px text-sm text-content-secondary no-underline font-medium py-2 pl-4 pr-3 transition-colors",
				"border-0 border-solid border-l border-l-transparent hover:text-content-primary",
				isActive &&
					"border-l-content-primary font-semibold text-content-primary",
			)
		}
	>
		{children}
	</NavLink>
);

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
						AI Gateway keys
					</SidebarNavItem>
				)}
				{permissions.viewAnyAIProvider && (
					<SidebarNavItem href="/ai/settings/providers">
						Providers
					</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<>
						<SidebarNavItem href="/ai/settings/coder-agents">
							Coder Agents
						</SidebarNavItem>
						<div className="flex flex-col gap-1 ml-3 border-0 border-solid border-l border-l-border">
							<SubNavItem href="/ai/settings/models">Models</SubNavItem>
							<SubNavItem href="/ai/settings/mcp-servers">
								MCP servers
							</SubNavItem>
							<SubNavItem href="/ai/settings/templates">Templates</SubNavItem>
							<SubNavItem href="/ai/settings/spend">Spend</SubNavItem>
							<SubNavItem href="/ai/settings/instructions">
								Instructions
							</SubNavItem>
							<SubNavItem href="/ai/settings/lifecycle">Lifecycle</SubNavItem>
						</div>
					</>
				)}
			</div>
		</BaseSidebar>
	);
};

export default AISettingsSidebarView;
