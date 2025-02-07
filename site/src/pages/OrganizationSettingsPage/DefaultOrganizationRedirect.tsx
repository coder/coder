import type { FC } from "react";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { Navigate } from "react-router-dom";
import { canEditOrganization } from "modules/management/organizationPermissions";

const DefaultOrganizationRedirect: FC = () => {
	const { organizations, permissionsByOrganizationId } =
		useOrganizationSettings();

	// Redirect /organizations => /organizations/default-org, or if they cannot edit
	// the default org, then the first org they can edit, if any.
	// .find will stop at the first match found; make sure default
	// organizations are placed first
	const editableOrg = [...organizations]
		.sort((a, b) => (b.is_default ? 1 : 0) - (a.is_default ? 1 : 0))
		.find((org) => canEditOrganization(permissionsByOrganizationId[org.id]));
	if (editableOrg) {
		return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
	}
	return <EmptyState message="No organizations found" />;
};

export default DefaultOrganizationRedirect;
