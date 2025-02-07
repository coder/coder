import {
	deleteOrganization,
	organizationsPermissions,
	updateOrganization,
} from "api/queries/organizations";
import type { AuthorizationResponse } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization?: string;
	};
	const { organizations } = useOrganizationSettings();

	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);
	const deleteOrganizationMutation = useMutation(
		deleteOrganization(queryClient),
	);

	const organization = organizations?.find((o) => o.name === organizationName);
	const permissionsQuery = useQuery(
		organizationsPermissions(organizations?.map((o) => o.id)),
	);

	if (permissionsQuery.isLoading) {
		return <Loader />;
	}

	const permissions = permissionsQuery.data;
	if (permissionsQuery.error || !permissions) {
		return <ErrorAlert error={permissionsQuery.error} />;
	}

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	// The user may not be able to edit this org but they can still see it because
	// they can edit members, etc.  In this case they will be shown a read-only
	// summary page instead of the settings form.
	// Similarly, if the feature is not entitled then the user will not be able to
	// edit the organization.
	if (!permissions[organization.id]?.editOrganization) {
		return <Navigate to=".." replace />;
	}

	const error =
		updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

	return (
		<OrganizationSettingsPageView
			organization={organization}
			error={error}
			onSubmit={async (values) => {
				const updatedOrganization =
					await updateOrganizationMutation.mutateAsync({
						organizationId: organization.id,
						req: values,
					});
				navigate(`/organizations/${updatedOrganization.name}/settings`);
				displaySuccess("Organization settings updated.");
			}}
			onDeleteOrganization={() => {
				deleteOrganizationMutation.mutate(organization.id);
				displaySuccess("Organization deleted.");
				navigate("/organizations");
			}}
		/>
	);
};

export default OrganizationSettingsPage;
