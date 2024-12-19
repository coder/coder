import { Loader } from "components/Loader/Loader";
import { type FC, Suspense } from "react";
import { Outlet } from "react-router-dom";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	return (
		<div className="flex flex-row gap-28 py-10">
			<OrganizationSidebar />
			<main css={{ flexGrow: 1 }}>
				<Suspense fallback={<Loader />}>
					<Outlet />
				</Suspense>
			</main>
		</div>
	);
};

export default OrganizationSidebarLayout;
