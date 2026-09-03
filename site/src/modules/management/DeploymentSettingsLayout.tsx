import { type FC, Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { canViewDeploymentSettings } from "#/modules/permissions";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { firstVisibleDeploymentPage } from "./adminNavigation";

const DeploymentSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const { entitlements, experiments, buildInfo } = useDashboard();
	const location = useLocation();

	if (location.pathname === "/deployment") {
		return (
			<Navigate
				to={firstVisibleDeploymentPage({
					permissions,
					hasPremiumLicense:
						entitlements.features.multiple_organizations.enabled,
					experiments,
					buildInfo,
				})}
				replace
			/>
		);
	}

	// The deployment settings page also contains users and groups and more so
	// this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage = canViewDeploymentSettings(permissions);

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<Suspense fallback={<Loader />}>
				<Outlet />
			</Suspense>
		</RequirePermission>
	);
};

export default DeploymentSettingsLayout;
