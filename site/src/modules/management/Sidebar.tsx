import { organizationsPermissions } from "api/queries/organizations";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	canEditOrganization,
	useManagementSettings,
} from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useLocation, useParams } from "react-router-dom";
import { type OrganizationWithPermissions, SidebarView } from "./SidebarView";

/**
 * A combined deployment settings and organization menu.
 *
 * This should only be used with multi-org support.  If multi-org support is
 * disabled or not licensed, this is the wrong sidebar to use.  See
 * DeploySettingsPage/Sidebar instead.
 */
export const Sidebar: FC = () => {
	const location = useLocation();
	const { permissions } = useAuthenticated();
	const { organizations } = useManagementSettings();
	const { organization: organizationName } = useParams() as {
		organization?: string;
	};

	const orgPermissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);

	// Sometimes a user can read an organization but cannot actually do anything
	// with it.  For now, these are filtered out so you only see organizations you
	// can manage in some way.
	const editableOrgs = organizations
		?.map((org) => {
			return {
				...org,
				permissions: orgPermissionsQuery.data?.[org.id],
			};
		})
		// TypeScript is not able to infer whether permissions are defined on the
		// object even if we explicitly check org.permissions here, so add the `is`
		// here to help out (canEditOrganization does the actual check).
		.filter((org): org is OrganizationWithPermissions => {
			return canEditOrganization(org.permissions);
		});

	return (
		<SidebarView
			// Both activeSettings and activeOrganizationName could be be falsey if
			// the user is on /organizations but has no editable organizations to
			// which we can redirect.
			activeSettings={location.pathname.startsWith("/deployment")}
			activeOrganizationName={organizationName}
			organizations={editableOrgs}
			permissions={permissions}
		/>
	);
};
