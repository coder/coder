import type { FC } from "react";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { OrganizationSidebarView } from "./OrganizationSidebarView";

/**
 * Sidebar for the OrganizationSidebarLayout
 */
export const OrganizationSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations, organization, organizationPermissions } =
		useOrganizationSettings();

	return (
		<OrganizationSidebarView
			activeOrganization={organization}
			orgPermissions={organizationPermissions}
			organizations={organizations}
			permissions={permissions}
		/>
	);
};
