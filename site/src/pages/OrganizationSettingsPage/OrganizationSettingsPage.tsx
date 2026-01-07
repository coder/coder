import { getErrorMessage } from "api/errors";
import {
	deleteOrganization,
	updateOrganization,
	updateWorkspaceSharingSettings,
	workspaceSharingSettings,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useDashboard } from "modules/dashboard/useDashboard";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { experiments } = useDashboard();

	const showWorkspaceSharingSettings =
		experiments.includes("workspace-sharing");

	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);
	const deleteOrganizationMutation = useMutation(
		deleteOrganization(queryClient),
	);

	const workspaceSharingSettingsQuery = useQuery({
		...workspaceSharingSettings(organization?.id ?? ""),
		enabled: showWorkspaceSharingSettings && !!organization,
	});

	const updateWorkspaceSharingMutation = useMutation({
		...updateWorkspaceSharingSettings(organization?.id ?? "", queryClient),
	});

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	const title = (
		<title>
			{pageTitle("Settings", organization.display_name || organization.name)}
		</title>
	);

	if (!organizationPermissions?.editSettings) {
		return (
			<>
				{title}
				<RequirePermission isFeatureVisible={false} />
			</>
		);
	}

	const error =
		updateOrganizationMutation.error ??
		deleteOrganizationMutation.error ??
		updateWorkspaceSharingMutation.error;

	return (
		<>
			{title}
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
				showWorkspaceSharingSettings={showWorkspaceSharingSettings}
				workspaceSharingSettings={workspaceSharingSettingsQuery.data}
				onUpdateWorkspaceSharingSettings={async (settings) => {
					await updateWorkspaceSharingMutation.mutateAsync(settings);
					displaySuccess(
						settings.sharing_disabled
							? "Workspace sharing disabled."
							: "Workspace sharing enabled.",
					);
				}}
				isUpdatingWorkspaceSharingSettings={
					updateWorkspaceSharingMutation.isPending
				}
				canEditWorkspaceSharingSettings={organizationPermissions?.editSettings}
			/>
		</>
	);
};

export default OrganizationSettingsPage;
