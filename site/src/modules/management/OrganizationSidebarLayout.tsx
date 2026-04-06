import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	return (
		<div className="flex flex-row flex-1 min-h-0 w-full">
			<OrganizationSidebar />
			<div className="flex flex-col items-center flex-1 min-h-0 h-full overflow-y-auto w-full px-10 pt-10">
				<Suspense fallback={<Loader />}>
					<Outlet />
				</Suspense>
			</div>
		</div>
	);
};

export default OrganizationSidebarLayout;
