import { getErrorMessage } from "api/errors";
import {
	deleteOrganization,
	updateOrganization,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { displayError } from "components/GlobalSnackbar/utils";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate } from "react-router-dom";
import { pageTitle } from "utils/page";
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

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const helmet = (
		<Helmet>
			<title>
				{pageTitle("Settings", organization.display_name || organization.name)}
			</title>
		</Helmet>
	);

	if (!organizationPermissions?.editSettings) {
		return (
			<>
				{helmet}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	const error =
		updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

	return (
		<>
			{helmet}
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
						displaySuccess("Organization deleted");
						navigate("/organizations");
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to delete organization"),
						);
					}
				}}
			/>
		</>
	);
};

export default OrganizationSettingsPage;
