import { type FC, useEffect } from "react";
import { useQueryClient } from "react-query";
import { useLocation } from "react-router";
import { entitlementsQueryKey } from "#/api/queries/entitlements";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { useCanShareOrganizationMCPServers } from "#/pages/AISettingsPage/MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { useCanViewAISpend } from "#/pages/AISettingsPage/SpendPage/spendAccess";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !permissions.editDeploymentConfig },
	);
	const spendAccess = useCanViewAISpend();

	// Entitlements are cached for the session, so without this a license
	// change would keep the AI Gateway entries until a full page load.
	const queryClient = useQueryClient();
	const { pathname } = useLocation();
	useEffect(() => {
		void queryClient.invalidateQueries({ queryKey: entitlementsQueryKey });
	}, [queryClient, pathname]);

	return (
		<AISettingsSidebarView
			permissions={permissions}
			canViewAISpend={spendAccess.canView}
			canAccessOrganizationModels={
				(accessibleOrgsQuery.organizations.length ?? 0) > 0
			}
			canShareOrganizationMCPServers={organizationMCPSharing.canShare}
		/>
	);
};
