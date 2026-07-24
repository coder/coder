import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	return (
		<section className="px-4 sm:px-6 lg:px-10 max-w-screen-2xl mx-auto">
			<div className="flex flex-col gap-8 py-6 lg:flex-row lg:gap-28 lg:py-10">
				<OrganizationSidebar />
				<div className="grow min-w-0">
					<Suspense fallback={<Loader />}>
						<Outlet />
					</Suspense>
				</div>
			</div>
		</section>
	);
};

export default OrganizationSidebarLayout;
