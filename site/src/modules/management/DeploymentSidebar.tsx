import { useAuthenticated } from "contexts/auth/RequireAuth";
import type { FC } from "react";
import { DeploymentSidebarView } from "./DeploymentSidebarView";
import { useDashboard } from "modules/dashboard/useDashboard";

/**
 * A sidebar for deployment settings.
 */
export const DeploymentSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, showOrganizations } = useDashboard();
	const hasPremiumLicense =
		entitlements.features.multiple_organizations.enabled;

	return (
		<DeploymentSidebarView
			permissions={permissions}
			showOrganizations={showOrganizations}
			hasPremiumLicense={hasPremiumLicense}
		/>
	);
};
