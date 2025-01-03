import { organizationsPermissions } from "api/queries/organizations";
import { useAuthenticated } from "contexts/auth/RequireAuth";
import {
	canEditOrganization,
	useOrganizationSettings,
} from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import {
	OrganizationSidebarView,
	type OrganizationWithPermissions,
} from "./OrganizationSidebarView";

/**
 * A combined deployment settings and organization menu.
 *
 * This should only be used with multi-org support.  If multi-org support is
 * disabled or not licensed, this is the wrong sidebar to use.  See
 * DeploySettingsPage/Sidebar instead.
 */
export const OrganizationSidebar: FC = () => {
	const { permissions } = useAuthenticated();
	const { organizations } = useOrganizationSettings();
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

	const organization = editableOrgs?.find((o) => o.name === organizationName);

	return (
		<OrganizationSidebarView
			activeOrganization={organization}
			organizations={editableOrgs}
			permissions={permissions}
		/>
	);
};
