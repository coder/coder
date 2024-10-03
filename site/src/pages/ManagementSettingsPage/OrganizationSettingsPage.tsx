import {
	deleteOrganization,
	organizationsPermissions,
	updateOrganization,
} from "api/queries/organizations";
import type { Organization } from "api/typesGenerated";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import {
	canEditOrganization,
	useManagementSettings,
} from "modules/management/ManagementSettingsLayout";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";
import { OrganizationSummaryPageView } from "./OrganizationSummaryPageView";

const OrganizationSettingsPage: FC = () => {
	const { organization: organizationName } = useParams() as {
		organization?: string;
	};
	const { organizations } = useManagementSettings();
	const feats = useFeatureVisibility();

	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);
	const deleteOrganizationMutation = useMutation(
		deleteOrganization(queryClient),
	);

	const organization =
		organizations && organizationName
			? getOrganizationByName(organizations, organizationName)
			: undefined;
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

	// Redirect /organizations => /organizations/default-org, or if they cannot edit
	// the default org, then the first org they can edit, if any.
	if (!organizationName) {
		const editableOrg = [...organizations]
			.sort((a, b) => {
				// Prefer default org (it may not be first).
				// JavaScript will happily subtract booleans, but use numbers to keep
				// the compiler happy.
				return (b.is_default ? 1 : 0) - (a.is_default ? 1 : 0);
			})
			.find((org) => canEditOrganization(permissions[org.id]));
		if (editableOrg) {
			return <Navigate to={`/organizations/${editableOrg.name}`} replace />;
		}
		return <EmptyState message="No organizations found" />;
	}

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	// The user may not be able to edit this org but they can still see it because
	// they can edit members, etc.  In this case they will be shown a read-only
	// summary page instead of the settings form.
	// Similarly, if the feature is not entitled then the user will not be able to
	// edit the organization.
	if (
		!permissions[organization.id]?.editOrganization ||
		!feats.multiple_organizations
	) {
		return <OrganizationSummaryPageView organization={organization} />;
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
				navigate(`/organizations/${updatedOrganization.name}`);
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

const getOrganizationByName = (
	organizations: readonly Organization[],
	name: string,
) => {
	return organizations.find((org) => org.name === name);
};
