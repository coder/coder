import { EmptyState } from "components/EmptyState/EmptyState";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { canEditOrganization } from "modules/permissions/organizations";
import type { FC } from "react";
import { Navigate } from "react-router-dom";

const OrganizationRedirect: FC = () => {
	const {
		organizations,
		organizationPermissionsByOrganizationId: organizationPermissions,
	} = useOrganizationSettings();

	const sortedOrganizations = [...organizations].sort(
		(a, b) => (b.is_default ? 1 : 0) - (a.is_default ? 1 : 0),
	);

	// Redirect /organizations => /organizations/some-organization-name
	// If they can edit the default org, we should redirect to the default.
	// If they cannot edit the default, we should redirect to the first org that
	// they can edit.
	const editableOrg = sortedOrganizations.find((org) =>
		canEditOrganization(organizationPermissions[org.id]),
	);
	if (editableOrg) {
		return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
	}
	// If they cannot edit any org, just redirect to an org they can read.
	if (sortedOrganizations.length > 0) {
		return (
			<Navigate to={`/organizations/${sortedOrganizations[0].name}`} replace />
		);
	}
	return <EmptyState message="No organizations found" />;
};

export default OrganizationRedirect;
