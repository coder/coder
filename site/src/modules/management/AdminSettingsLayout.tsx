import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { CollapsibleSidebar } from "#/components/Sidebar/CollapsibleSidebar";
import { AdminSettingsSidebar } from "./AdminSettingsSidebar";

/**
 * Shared shell for every admin settings area. Renders the unified
 * collapsible sidebar beside the section content.
 */
const AdminSettingsLayout: FC = () => {
	return (
		<div className="flex flex-row min-h-screen">
			<div className="relative border-0 border-r border-solid border-border">
				<CollapsibleSidebar storageKey="admin-sidebar-width">
					<AdminSettingsSidebar />
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
	);
};

export default AdminSettingsLayout;
