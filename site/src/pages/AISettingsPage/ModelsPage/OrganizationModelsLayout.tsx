import type { FC } from "react";
import { useQuery } from "react-query";
import { Outlet, useParams } from "react-router";
import { organizationsPermissions } from "#/api/queries/organizations";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { Loader } from "#/components/Loader/Loader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { OrganizationModelsSwitcher } from "./OrganizationModelsSwitcher";
import { OrganizationModelsContext } from "./organizationModels";

/**
 * Org context for the /ai/settings models pages. Resolves the :organization
 * route param against the organizations the caller may manage chat model
 * configs in, and renders an org switcher above the matched page.
 */
const OrganizationModelsLayout: FC = () => {
	const { organization } = useParams<{ organization: string }>();
	const { organizations } = useDashboard();

	const manageableOrgsQuery = useQuery({
		...organizationsPermissions(
			organizations.length > 0 ? organizations.map((org) => org.id) : undefined,
		),
		select: (permissionsByOrg) =>
			organizations.filter(
				(org) =>
					permissionsByOrg[org.id]?.createChatModelConfigs ||
					permissionsByOrg[org.id]?.editChatModelConfigs,
			),
	});

	const manageableOrganizations = manageableOrgsQuery.data ?? [];
	const activeOrganization = manageableOrganizations.find(
		(org) => org.name === organization,
	);

	if (manageableOrgsQuery.isLoading) {
		return <Loader />;
	}

	if (manageableOrgsQuery.error !== null) {
		return <ErrorAlert error={manageableOrgsQuery.error} />;
	}

	if (activeOrganization === undefined) {
		return (
			<EmptyState
				message="No manageable organizations"
				description="You do not have permission to manage chat models in any organization. Ask an organization administrator for access."
			/>
		);
	}

	return (
		<OrganizationModelsContext.Provider
			value={{
				organization: activeOrganization,
				organizations: manageableOrganizations,
			}}
		>
			<div className="flex flex-col gap-6">
				<div>
					<OrganizationModelsSwitcher
						activeOrganization={activeOrganization}
						organizations={manageableOrganizations}
					/>
				</div>
				<Outlet />
			</div>
		</OrganizationModelsContext.Provider>
	);
};

export default OrganizationModelsLayout;
