import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { useAuthenticated } from "hooks";
import { type FC, Suspense } from "react";
import { Outlet } from "react-router";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
	const { user: me } = useAuthenticated();

	return (
		<>
			<title>{pageTitle("Settings")}</title>

			<Margins>
				<div className="flex flex-col md:flex-row gap-12 py-12">
					<Sidebar user={me} />
					<Suspense fallback={<Loader />}>
						<main className="max-w-[800px] w-full">
							<Outlet />
						</main>
					</Suspense>
				</div>
			</Margins>
		</>
	);
};

export default Layout;
