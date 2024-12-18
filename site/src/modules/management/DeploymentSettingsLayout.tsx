import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { RequirePermission } from "contexts/auth/RequirePermission";
import { type FC, Suspense } from "react";
import { Outlet } from "react-router-dom";
import { DeploymentSidebar } from "./DeploymentSidebar";

const DeploymentSettingsLayout: FC = () => {
	const { permissions } = useAuthenticated();

	// The deployment settings page also contains users, audit logs, and groups
	// so this page must be visible if you can see any of these.
	const canViewDeploymentSettingsPage =
		permissions.viewDeploymentValues ||
		permissions.viewAllUsers ||
		permissions.viewAnyAuditLog;

	return (
		<RequirePermission isFeatureVisible={canViewDeploymentSettingsPage}>
			<div className="px-10 max-w-screen-2xl">
				<div className="flex flex-row gap-12 py-10">
					<DeploymentSidebar />
					<main css={{ flexGrow: 1 }}>
						<Suspense fallback={<Loader />}>
							<Outlet />
						</Suspense>
					</main>
				</div>
			</div>
		</RequirePermission>
	);
};

export default DeploymentSettingsLayout;
