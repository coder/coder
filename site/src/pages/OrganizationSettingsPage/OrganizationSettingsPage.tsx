import { getErrorMessage } from "api/errors";
import {
	deleteOrganization,
	updateOrganization,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { displayError } from "components/GlobalSnackbar/utils";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import type { FC } from "react";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization, organizationPermissions } = useOrganizationSettings();

	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);
	const deleteOrganizationMutation = useMutation(
		deleteOrganization(queryClient),
	);

	if (!organization || !organizationPermissions?.editSettings) {
		return <EmptyState message="Organization not found" />;
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
			onDeleteOrganization={async () => {
				try {
					await deleteOrganizationMutation.mutateAsync(organization.id);
					displaySuccess(`Organization ${organization.name} deleted`);
					navigate("/organizations");
				} catch (error) {
					displayError(
						getErrorMessage(
							error,
							`Failed to delete organization: ${organization.name}`,
						),
					);
				}
			}}
		/>
	);
};

export default OrganizationSettingsPage;
