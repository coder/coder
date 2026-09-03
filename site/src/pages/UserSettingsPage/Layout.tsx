import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	DEPLOYMENT_BANNER_HEIGHT,
	useIsDeploymentBannerVisible,
} from "#/modules/dashboard/DeploymentBanner/DeploymentBanner";
import { pageTitle } from "#/utils/page";
import { UserSettingsSidebar } from "./UserSettingsSidebar";
import { UserSettingsSidebarHeader } from "./UserSettingsSidebarView";

const Layout: FC = () => {
	const { user: me } = useAuthenticated();
	const isBannerVisible = useIsDeploymentBannerVisible();

	return (
		<>
			<title>{pageTitle("Settings")}</title>

			<div className="flex flex-row min-h-screen">
				<div className="relative z-30 border-0 border-r border-solid border-border">
					<CollapsibleSidebar
						storageKey="user-settings-sidebar-width"
						header={<UserSettingsSidebarHeader user={me} />}
						bottomInset={isBannerVisible ? DEPLOYMENT_BANNER_HEIGHT : 0}
					>
						<UserSettingsSidebar />
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
		</>
	);
};

export default Layout;
