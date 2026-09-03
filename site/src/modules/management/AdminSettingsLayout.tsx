import { type FC, Suspense } from "react";
import { Outlet, useLocation } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import {
	DEPLOYMENT_BANNER_HEIGHT,
	useIsDeploymentBannerVisible,
} from "#/modules/dashboard/DeploymentBanner/DeploymentBanner";
import { AdminSettingsSidebar } from "./AdminSettingsSidebar";
import { AdminSettingsSidebarHeader } from "./AdminSettingsSidebarView";
import { isWideContentRoute } from "./adminNavigation";

/**
 * Shared shell for every admin settings area. Renders the unified
 * collapsible sidebar beside the section content. On wide-content pages
 * the sidebar settles collapsed so tables get the full width.
 */
const AdminSettingsLayout: FC = () => {
	const { pathname } = useLocation();
	const isBannerVisible = useIsDeploymentBannerVisible();

	return (
		<div className="flex flex-row min-h-screen">
			<div className="relative z-30 border-0 border-r border-solid border-border">
				<CollapsibleSidebar
					storageKey="admin-sidebar-width"
					header={<AdminSettingsSidebarHeader />}
					bottomInset={isBannerVisible ? DEPLOYMENT_BANNER_HEIGHT : 0}
					preferCollapsed={isWideContentRoute(pathname)}
				>
					<AdminSettingsSidebar />
				</CollapsibleSidebar>
			</div>
			<div className="flex-1 min-w-0 pt-6 pb-10 px-4 sm:px-6 lg:px-10">
				<div className="max-w-(--breakpoint-2xl) mx-auto">
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</div>
			</div>
		</div>
	);
};

export default AdminSettingsLayout;
