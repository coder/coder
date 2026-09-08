import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { DeploymentSidebarView } from "./DeploymentSidebarView";

/**
 * A sidebar for deployment settings.
 */
export const DeploymentSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, showOrganizations, experiments, buildInfo } =
		useDashboard();
	// Trialing deployments keep the Premium tab so they can convert.
	const hidePremiumTab = entitlements.has_license && !entitlements.trial;

	return (
		<DeploymentSidebarView
			permissions={permissions}
			showOrganizations={showOrganizations}
			hidePremiumTab={hidePremiumTab}
			experiments={experiments}
			buildInfo={buildInfo}
		/>
	);
};
