import { useAuthenticated } from "hooks";
import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { pageTitle } from "utils/page";
import { Loader } from "#/components/Loader/Loader";
import { Margins } from "#/components/Margins/Margins";
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
