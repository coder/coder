import type { FC } from "react";
import { SidebarLayout } from "#/components/Sidebar";
import { HealthSidebar } from "#/modules/management/HealthSidebar";
import { pageTitle } from "#/utils/page";

export const HealthLayout: FC = () => {
	return (
		<>
			<title>{pageTitle("Health")}</title>
			<SidebarLayout sidebar={<HealthSidebar />} />
		</>
	);
};
