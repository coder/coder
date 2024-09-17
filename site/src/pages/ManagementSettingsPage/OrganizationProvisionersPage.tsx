import {
	organizationsPermissions,
	provisionerDaemons,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import NotFoundPage from "pages/404Page/404Page";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

const OrganizationProvisionersPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organizations } = useOrganizationSettings();

	const organization = organizations
		? getOrganizationByName(organizations, organizationName)
		: undefined;
	const permissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);
	const provisionersQuery = useQuery(provisionerDaemons(organizationName));

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	if (permissionsQuery.isLoading || provisionersQuery.isLoading) {
		return <Loader />;
	}

	const permissions = permissionsQuery.data;
	const provisioners = provisionersQuery.data;
	const error = permissionsQuery.error || provisionersQuery.error;
	if (error || !permissions || !provisioners) {
		return <ErrorAlert error={error} />;
	}

	// The user may not be able to edit this org but they can still see it because
	// they can edit members, etc.  In this case they will be shown a read-only
	// summary page instead of the settings form.
	// Similarly, if the feature is not entitled then the user will not be able to
	// edit the organization.
	if (!permissions[organization.id]?.viewProvisioners) {
		// This probably doesn't work with the layout................fix this pls
		// Kayla, hey, yes you, you gotta fix this.
		// Don't scroll past this. It's important. Fix it!!!
		return <NotFoundPage />;
	}

	return <OrganizationProvisionersPageView provisioners={provisioners} />;
};

export default OrganizationProvisionersPage;

const getOrganizationByName = (
	organizations: readonly Organization[],
	name: string,
) => {
	return organizations.find((org) => org.name === name);
};
