import type { FC } from "react";
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
