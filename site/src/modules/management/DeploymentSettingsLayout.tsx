import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "components/Breadcrumb/Breadcrumb";
import { Loader } from "components/Loader/Loader";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { canViewDeploymentSettings } from "modules/permissions";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { type FC, Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
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
			<div>
				<Breadcrumb>
					<BreadcrumbList>
						<BreadcrumbItem>
							<BreadcrumbPage>Admin Settings</BreadcrumbPage>
						</BreadcrumbItem>
						<BreadcrumbSeparator />
						<BreadcrumbItem>
							<BreadcrumbPage className="text-content-primary">
								Deployment
							</BreadcrumbPage>
						</BreadcrumbItem>
					</BreadcrumbList>
				</Breadcrumb>
				<hr className="h-px border-none bg-border" />
				<div className="px-10 max-w-screen-2xl">
					<div className="flex flex-row gap-28 py-10">
						<DeploymentSidebar />
						<main css={{ flexGrow: 1 }}>
							<Suspense fallback={<Loader />}>
								<Outlet />
							</Suspense>
						</main>
					</div>
				</div>
			</div>
		</RequirePermission>
	);
};

export default DeploymentSettingsLayout;
