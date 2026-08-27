import { useQuery } from "react-query";
import { buildInfo } from "#/api/queries/buildInfo";
import type { LinkConfig } from "#/api/typesGenerated";
import { useProxy } from "#/contexts/ProxyContext";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useEmbeddedMetadata } from "#/hooks/useEmbeddedMetadata";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	canAccessAnyChatModelConfig,
	canViewDeploymentSettings,
	canViewTemplates,
	canViewWorkspaces,
} from "#/modules/permissions";
import { useCanShareOrganizationMCPServers } from "#/pages/AISettingsPage/MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { useFeatureVisibility } from "../useFeatureVisibility";
import { NavbarView } from "./NavbarView";

export const Navbar: React.FC = () => {
	const { metadata } = useEmbeddedMetadata();
	const buildInfoQuery = useQuery(buildInfo(metadata["build-info"]));
	const { appearance, canViewOrganizationSettings, organizations } =
		useDashboard();
	const { user: me, permissions, signOut } = useAuthenticated();
	const featureVisibility = useFeatureVisibility();
	const proxyContextValue = useProxy();
	const accessibleModelOrgsQuery =
		useAccessibleModelOrganizations(organizations);
	const canAccessAnyModel = canAccessAnyChatModelConfig(permissions);

	const canViewDeployment = canViewDeploymentSettings(permissions);
	const canViewOrganizations = canViewOrganizationSettings;
	const canViewHealth = permissions.viewDebugInfo;
	const canViewAuditLog =
		featureVisibility.audit_log && permissions.viewAnyAuditLog;
	const canViewConnectionLog =
		featureVisibility.connection_log && permissions.viewAnyConnectionLog;
	const canViewAIBridge =
		featureVisibility.aibridge && permissions.viewAnyAIBridgeInterception;
	const canViewSiteWideAISettings =
		permissions.viewAnyAIProvider ||
		permissions.viewAIGatewayKeys ||
		permissions.editDeploymentConfig ||
		permissions.viewAnyMCPServerConfigs ||
		permissions.createAnyMCPServerConfig ||
		permissions.updateAnyMCPServerConfig ||
		permissions.deleteAnyMCPServerConfig ||
		canAccessAnyModel;
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !canViewSiteWideAISettings },
	);
	const canViewAISettings =
		canViewSiteWideAISettings || organizationMCPSharing.canShare;
	const canViewModels =
		!canViewAISettings && accessibleModelOrgsQuery.organizations.length > 0;
	const canCreateChat = permissions.createChat;
	const canViewWorkspacesNav = canViewWorkspaces(permissions);
	const canViewTemplatesNav = canViewTemplates(permissions);

	const uniqueLinks = new Map<string, LinkConfig>();
	for (const link of appearance.support_links ?? []) {
		if (!uniqueLinks.has(link.name)) {
			uniqueLinks.set(link.name, link);
		}
	}
	return (
		<NavbarView
			user={me}
			buildInfo={buildInfoQuery.data}
			supportLinks={Array.from(uniqueLinks.values())}
			onSignOut={signOut}
			adminPermissions={{
				canViewDeployment,
				canViewOrganizations,
				canViewAISettings,
				canViewAuditLog,
				canViewConnectionLog,
				canViewAIBridge,
				canViewHealth,
			}}
			canViewModels={canViewModels}
			canCreateChat={canCreateChat}
			canViewWorkspaces={canViewWorkspacesNav}
			canViewTemplates={canViewTemplatesNav}
			proxyContextValue={proxyContextValue}
		/>
	);
};
