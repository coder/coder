import type { FC } from "react";
import { Navigate } from "react-router";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { canEditOrganization } from "#/modules/permissions/organizations";

const OrganizationRedirect: FC = () => {
	const {
		organizations,
		organizationPermissionsByOrganizationId: organizationPermissions,
	} = useOrganizationSettings();

	// Redirect /organizations => /organizations/some-organization-name
	// If they can edit the default org, we should redirect to the default.
	// If they cannot edit the default, we should redirect to the first org
	// that they can edit.
	const defaultOrg = organizations.find((org) => org.is_default);
	const editableOrg =
		(defaultOrg &&
			canEditOrganization(organizationPermissions[defaultOrg.id]) &&
			defaultOrg) ||
		organizations.find((org) =>
			canEditOrganization(organizationPermissions[org.id]),
		);
	if (editableOrg) {
		return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
	}
	// If they cannot edit any org, just redirect to one they can read.
	const viewableOrg = defaultOrg ?? organizations[0];
	if (viewableOrg) {
		return <Navigate to={`/organizations/${viewableOrg.name}`} replace />;
	}
	return <EmptyState message="No organizations found" />;
};

export default OrganizationRedirect;
