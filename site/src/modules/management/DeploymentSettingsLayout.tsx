import { type FC, Suspense } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
	BreadcrumbSeparator,
} from "#/components/Breadcrumb/Breadcrumb";
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
				<section className="px-6 max-w-screen-2xl mx-auto">
					<div className="flex flex-row py-10">
						<CollapsibleSidebar storageKey="deployment-sidebar-width">
							<DeploymentSidebar />
						</CollapsibleSidebar>
						<div className="flex-1 min-w-0 pl-10">
							<Suspense fallback={<Loader />}>
								<Outlet />
							</Suspense>
						</div>
					</div>
				</section>
			</div>
		</RequirePermission>
	);
};

export default DeploymentSettingsLayout;
