import { useAuthenticated } from "hooks";
import { useDashboard } from "modules/dashboard/useDashboard";
import type { FC } from "react";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

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
