import { EmptyState } from "components/EmptyState/EmptyState";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { canEditOrganization } from "modules/management/organizationPermissions";
import type { FC } from "react";
import { Navigate } from "react-router-dom";

const DefaultOrganizationRedirect: FC = () => {
	const {
		organizations,
		organizationPermissionsByOrganizationId: organizationPermissions,
	} = useOrganizationSettings();

	// Redirect /organizations => /organizations/some-organization-name
	// If they can edit the default org, we should redirect to the default.
	// If they cannot edit the default, we should redirect to the first org that
	// they can edit.
	const editableOrg = [...organizations]
		.sort((a, b) => (b.is_default ? 1 : 0) - (a.is_default ? 1 : 0))
		.find((org) => canEditOrganization(organizationPermissions[org.id]));
	if (editableOrg) {
		return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
	}
	// If they cannot edit any org, just redirect to an org they can read.
	if (organizations.length > 0) {
		return <Navigate to={`/organizations/${organizations[0].name}`} replace />;
	}
	return <EmptyState message="No organizations found" />;
};

export default DefaultOrganizationRedirect;
