import {
	organizationsPermissions,
	provisionerDaemons,
} from "api/queries/organizations";
import type { Organization, ProvisionerDaemon } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { Loader } from "components/Loader/Loader";
import NotFoundPage from "pages/404Page/404Page";
import type { FC } from "react";
import { useQuery } from "react-query";
import { useParams } from "react-router-dom";
import { useOrganizationSettings } from "./ManagementSettingsLayout";
import { OrganizationProvisionersPageView } from "./OrganizationProvisionersPageView";

export interface ProvisionersByGroup {
	builtin: ProvisionerDaemon[];
	psk: ProvisionerDaemon[];
	keys: Map<string, ProvisionerDaemon[]>;
}

function groupProvisioners(
	provisioners: readonly ProvisionerDaemon[],
): ProvisionersByGroup {
	const groups: ProvisionersByGroup = { builtin: [], psk: [], keys: new Map() };
	const type = "builtin";
	const keyName = "TODO";

	for (const it of provisioners) {
		if (type === "builtin") {
			groups.builtin.push(it);
			continue;
		}
		if (type === "psk") {
			groups.psk.push(it);
			continue;
		}

		const keyGroup = groups.keys.get(keyName) ?? [];
		if (!groups.keys.has(keyName)) {
			groups.keys.set(keyName, keyGroup);
		}
		keyGroup.push(it);
	}

	return groups;
}

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

	return (
		<OrganizationProvisionersPageView
			provisioners={groupProvisioners(provisioners)}
		/>
	);
};

export default OrganizationProvisionersPage;

const getOrganizationByName = (organizations: Organization[], name: string) =>
	organizations.find((org) => org.name === name);
