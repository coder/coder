import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import AISettingsSidebarView from "#/modules/management/AISettingsSidebarView";
import { useAccessibleModelOrganizations } from "#/pages/AISettingsPage/ModelsPage/organizationModels";

/**
 * A sidebar for AI settings.
 */
export const AISettingsSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useDashboard();
	const accessibleOrgsQuery = useAccessibleModelOrganizations(organizations);

	return (
		<AISettingsSidebarView
			permissions={permissions}
			canAccessOrganizationModels={
				(accessibleOrgsQuery.organizations.length ?? 0) > 0
			}
		/>
	);
};
