import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import {
	Breadcrumb,
	BreadcrumbItem,
	BreadcrumbList,
	BreadcrumbPage,
} from "#/components/Breadcrumb/Breadcrumb";
import { Loader } from "#/components/Loader/Loader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
	const { user: me } = useAuthenticated();

	return (
		<>
			<title>{pageTitle("Settings")}</title>

			<div>
				<Breadcrumb>
					<BreadcrumbList>
						<BreadcrumbItem>
							<BreadcrumbPage className="text-content-primary">
								User Settings
							</BreadcrumbPage>
						</BreadcrumbItem>
					</BreadcrumbList>
				</Breadcrumb>
				<div className="h-px border-none bg-border" />
				<section className="px-10 max-w-screen-2xl mx-auto">
					<div className="flex flex-row gap-28 py-10">
						<Sidebar user={me} />
						<div className="grow">
							<Suspense fallback={<Loader />}>
								<Outlet />
							</Suspense>
						</div>
					</div>
				</section>
			</div>
		</>
	);
};

export default Layout;
