import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useAccessibleModelOrganizations } from "#/modules/aiModels/organizationModels";
import { useCanShareOrganizationMCPServers } from "#/modules/aiSettings/organizationSharing";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";

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

	return (
		<AISettingsSidebarView
			permissions={permissions}
			canAccessOrganizationModels={
				(accessibleOrgsQuery.organizations.length ?? 0) > 0
			}
			canShareOrganizationMCPServers={organizationMCPSharing.canShare}
		/>
	);
};
