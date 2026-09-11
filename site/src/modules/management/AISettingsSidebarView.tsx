import { cn } from "cn";
import type { FC, ReactNode } from "react";
import {
	Link,
	NavLink,
	type To,
	useMatch,
	useSearchParams,
} from "react-router";
import {
	Sidebar as BaseSidebar,
	SettingsSidebarNavItem as SidebarNavItem,
} from "#/components/Sidebar/Sidebar";
import {
	canAccessAnyChatModelConfig,
	type Permissions,
} from "#/modules/permissions";
import { modelOrganizationSearchParam } from "#/pages/AISettingsPage/ModelsPage/organizationModels";

interface AISettingsSidebarViewProps {
	/** Site-wide permissions. */
	permissions: Permissions;
	canViewAISpend?: boolean;
	canAccessOrganizationModels?: boolean;
	canShareOrganizationMCPServers?: boolean;
}

const SubNavItem: FC<{ href: To; children?: ReactNode }> = ({
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

const organizationScopedPath = (
	pathname: string,
	organizationName: string | null,
): To => ({
	pathname,
	search: organizationName
		? new URLSearchParams({
				[modelOrganizationSearchParam]: organizationName,
			}).toString()
		: "",
});

const ModelsSidebarNavItem: FC<{ href: To }> = ({ href }) => {
	const legacyMatch = useMatch("/ai/settings/models/*");
	const organizationMatch = useMatch(
		"/ai/settings/organizations/:organization/models/*",
	);
	const isActive = legacyMatch !== null || organizationMatch !== null;

	return (
		<Link
			to={href}
			aria-current={isActive ? "page" : undefined}
			className={cn(
				"relative text-sm text-content-secondary no-underline font-medium py-2 px-3 hover:bg-surface-secondary rounded-md transition ease-in-out duration-150",
				isActive && "font-semibold text-content-primary",
			)}
		>
			Models
		</Link>
	);
};

const AISettingsSidebarView: FC<AISettingsSidebarViewProps> = ({
	permissions,
	canViewAISpend = false,
	canAccessOrganizationModels = false,
	canShareOrganizationMCPServers = false,
}) => {
	const [searchParams] = useSearchParams();
	const organizationName = searchParams.get(modelOrganizationSearchParam);
	const modelsPath = organizationScopedPath(
		"/ai/settings/models",
		organizationName,
	);
	const coderAgentsPath = organizationScopedPath(
		"/ai/settings/coder-agents",
		organizationName,
	);
	const mcpServersPath = organizationScopedPath(
		"/ai/settings/mcp-servers",
		organizationName,
	);
	const addMCPServerPath = organizationScopedPath(
		"/ai/settings/mcp-servers/add",
		organizationName,
	);

	return (
		<BaseSidebar>
			<div className="flex flex-col gap-1">
				{permissions.viewDeploymentConfig && (
					<SidebarNavItem href="/ai/settings/governance">
						AI Governance
					</SidebarNavItem>
				)}
				{canViewAISpend && (
					<SidebarNavItem href="/ai/settings/spend">Spend</SidebarNavItem>
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
				{(canAccessAnyChatModelConfig(permissions) ||
					canAccessOrganizationModels) && (
					<ModelsSidebarNavItem href={modelsPath} />
				)}
				{(permissions.editDeploymentConfig || canAccessOrganizationModels) && (
					<SidebarNavItem href={coderAgentsPath}>Coder Agents</SidebarNavItem>
				)}
				{permissions.editDeploymentConfig && (
					<div className="flex flex-col gap-1 ml-3 border-0 border-solid border-l border-l-border">
						<SubNavItem href={mcpServersPath}>MCP servers</SubNavItem>
						{permissions.updateAnyTemplate && (
							<SubNavItem href="/ai/settings/templates">Templates</SubNavItem>
						)}
						<SubNavItem href="/ai/settings/instructions">
							Instructions
						</SubNavItem>
						<SubNavItem href="/ai/settings/lifecycle">Lifecycle</SubNavItem>
					</div>
				)}
				{!permissions.editDeploymentConfig &&
					(permissions.viewAnyMCPServerConfigs ||
						permissions.createAnyMCPServerConfig ||
						permissions.updateAnyMCPServerConfig ||
						permissions.deleteAnyMCPServerConfig ||
						canShareOrganizationMCPServers) && (
						<div className="flex flex-col gap-1 ml-3 border-0 border-solid border-l border-l-border">
							<SubNavItem
								href={
									permissions.viewAnyMCPServerConfigs ||
									permissions.updateAnyMCPServerConfig ||
									permissions.deleteAnyMCPServerConfig ||
									canShareOrganizationMCPServers
										? mcpServersPath
										: addMCPServerPath
								}
							>
								MCP servers
							</SubNavItem>
						</div>
					)}
				{!permissions.editDeploymentConfig && permissions.updateAnyTemplate && (
					<div className="flex flex-col gap-1 ml-3 border-0 border-solid border-l border-l-border">
						<SubNavItem href="/ai/settings/templates">Templates</SubNavItem>
					</div>
				)}
			</div>
		</BaseSidebar>
	);
};

export default AISettingsSidebarView;
