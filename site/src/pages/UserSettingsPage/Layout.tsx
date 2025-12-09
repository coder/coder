import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
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
				<Stack className="py-12 px-0" direction="row" spacing={6}>
					<Sidebar user={me} />
					<Suspense fallback={<Loader />}>
						<main className="max-w-[800px] w-full">
							<Outlet />
						</main>
					</Suspense>
				</Stack>
			</Margins>
		</>
	);
};

export default Layout;
