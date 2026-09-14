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
	// Prefer the editable default org, then any editable org, then
	// any viewable org. This replaces the previous [...organizations]
	// .sort() approach with direct lookups to avoid copying the array.
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
