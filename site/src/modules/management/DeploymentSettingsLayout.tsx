import { type FC, Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { canViewDeploymentSettings } from "#/modules/permissions";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { DeploymentSidebar } from "./DeploymentSidebar";

const DeploymentSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();
	const location = useLocation();

	if (location.pathname === "/deployment") {
		return (
			<Navigate
				to={
					permissions.viewDeploymentConfig
						? "/deployment/overview"
						: "/deployment/users"
				}
				replace
			/>
		);
	}

	// The deployment settings page also contains users and groups and more so
	// this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage = canViewDeploymentSettings(permissions);

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<div className="flex flex-row min-h-screen">
				<div className="border-0 border-r border-solid border-border">
					<CollapsibleSidebar storageKey="deployment-sidebar-width">
						<DeploymentSidebar />
					</CollapsibleSidebar>
				</div>
				<div className="flex-1 min-w-0 pt-6 pb-10 px-10">
					<div className="max-w-screen-2xl mx-auto">
						<Suspense fallback={<Loader />}>
							<Outlet />
						</Suspense>
					</div>
				</div>
			</div>
		</RequirePermission>
	);
};

export default DeploymentSettingsLayout;
