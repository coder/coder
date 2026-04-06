import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { pageTitle } from "#/utils/page";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
	const { user: me } = useAuthenticated();

	return (
		<>
			<title>{pageTitle("Settings")}</title>

			<Margins>
				<div className="flex flex-row gap-12 py-12">
					<Sidebar user={me} />
					<Suspense fallback={<Loader />}>
						<div className="w-full max-w-full">
							<Outlet />
						</div>
					</Suspense>
				</div>
			</Margins>
		</>
	);
};

export default Layout;
