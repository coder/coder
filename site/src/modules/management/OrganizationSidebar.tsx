import { Sidebar as BaseSidebar } from "components/Sidebar/Sidebar";
import { useAuthenticated } from "hooks";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { OrganizationSidebarView } from "./OrganizationSidebarView";

/**
 * Sidebar for the OrganizationSettingsLayout
 */
export const OrganizationSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations, organization, organizationPermissions } =
		useOrganizationSettings();

	return (
		<BaseSidebar>
			<OrganizationSidebarView
				activeOrganization={organization}
				orgPermissions={organizationPermissions}
				organizations={organizations}
				permissions={permissions}
			/>
		</BaseSidebar>
	);
};
