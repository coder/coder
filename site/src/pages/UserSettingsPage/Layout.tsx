import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { Stack } from "components/Stack/Stack";
import { useAuthenticated } from "hooks";
import { type FC, Suspense } from "react";
import { Helmet } from "react-helmet-async";
import { Outlet } from "react-router-dom";
import { pageTitle } from "utils/page";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
	const { user: me } = useAuthenticated();

	return (
		<>
			<Helmet>
				<title>{pageTitle("Settings")}</title>
			</Helmet>

			<Margins>
				<Stack css={{ padding: "48px 0" }} direction="row" spacing={6}>
					<Sidebar user={me} />
					<Suspense fallback={<Loader />}>
						<main css={{ maxWidth: 800, width: "100%" }}>
							<Outlet />
						</main>
					</Suspense>
				</Stack>
			</Margins>
		</>
	);
};

export default Layout;
