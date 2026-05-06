import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	return (
		<section className="px-10 max-w-screen-2xl mx-auto">
			<div className="flex flex-row gap-28 py-10">
				<OrganizationSidebar />
				<div className="grow">
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</div>
			</div>
		</section>
	);
};

export default OrganizationSidebarLayout;
