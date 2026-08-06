import type { FC } from "react";
import { SidebarLayout } from "#/components/Sidebar";
import { pageTitle } from "#/utils/page";
import { Sidebar } from "./Sidebar";

const Layout: FC = () => {
	return (
		<>
			<title>{pageTitle("Settings")}</title>
			<SidebarLayout sidebar={<Sidebar />} />
		</>
	);
};

export default Layout;
