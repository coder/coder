import type { FC } from "react";
import { SidebarLayout } from "#/components/Sidebar";
import { OrganizationSidebar } from "./OrganizationSidebar";

const OrganizationSidebarLayout: FC = () => {
	return <SidebarLayout sidebar={<OrganizationSidebar />} />;
};

export default OrganizationSidebarLayout;
