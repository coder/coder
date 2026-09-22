import type { FC } from "react";
import { useQuery } from "react-query";
import { aiSpendOrganizations } from "#/api/queries/aiBridge";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { useCanShareOrganizationMCPServers } from "#/pages/AISettingsPage/MCPServersPage/organizationSharing";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";
import { canViewAISpend } from "#/pages/AISettingsPage/SpendPage/spendAccess";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);
	const organizationMCPSharing = useCanShareOrganizationMCPServers(
		organizations,
		{ enabled: !permissions.editDeploymentConfig },
	);
	const spendOrganizationsQuery = useQuery({
		...aiSpendOrganizations(),
		enabled: entitlements.features.aibridge.enabled,
	});

	return (
		<AISettingsSidebarView
			permissions={permissions}
			canViewAISpend={canViewAISpend(
				entitlements,
				spendOrganizationsQuery.data,
			)}
			canAccessOrganizationModels={
				(accessibleOrgsQuery.organizations.length ?? 0) > 0
			}
			canShareOrganizationMCPServers={organizationMCPSharing.canShare}
		/>
	);
};
