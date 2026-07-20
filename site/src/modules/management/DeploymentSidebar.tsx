import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { DeploymentSidebarView } from "./DeploymentSidebarView";
import { useActiveDeploymentSection } from "./useActiveDeploymentSection";

/**
 * A sidebar for deployment settings. Wraps the view with data from
 * authentication and dashboard context.
 */
export const DeploymentSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, showOrganizations, experiments, buildInfo } =
		useDashboard();
	const hasPremiumLicense =
		entitlements.features.multiple_organizations.enabled;
	const activeSection = useActiveDeploymentSection();

	return (
		<DeploymentSidebarView
			permissions={permissions}
			showOrganizations={showOrganizations}
			hasPremiumLicense={hasPremiumLicense}
			experiments={experiments}
			buildInfo={buildInfo}
			activeSection={activeSection}
		/>
	);
};
