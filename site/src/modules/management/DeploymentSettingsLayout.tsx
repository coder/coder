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
				<div className="h-px border-none bg-border" />
				<section className="px-4 sm:px-6 lg:px-10 max-w-screen-2xl mx-auto">
					<div className="flex flex-col gap-8 py-6 lg:flex-row lg:gap-28 lg:py-10">
						<DeploymentSidebar />
						<div className="grow min-w-0">
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
