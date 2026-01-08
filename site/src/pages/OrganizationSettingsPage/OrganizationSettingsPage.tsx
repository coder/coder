import { getErrorMessage } from "api/errors";
import {
	deleteOrganization,
	patchWorkspaceSharingSettings,
	updateOrganization,
	workspaceSharingSettings,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import { type FC, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { pageTitle } from "utils/page";
import { DisableWorkspaceSharingDialog } from "./DisableWorkspaceSharingDialog";
import { OrganizationSettingsPageView } from "./OrganizationSettingsPageView";

const OrganizationSettingsPage: FC = () => {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const { organization, organizationPermissions } = useOrganizationSettings();
	const [isDisableDialogOpen, setIsDisableDialogOpen] = useState(false);

	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);
	const deleteOrganizationMutation = useMutation(
		deleteOrganization(queryClient),
	);

	const sharingSettingsQuery = useQuery({
		...workspaceSharingSettings(organization?.id ?? ""),
		enabled: !!organization,
	});

	const patchSharingSettingsMutation = useMutation(
		patchWorkspaceSharingSettings(organization?.id ?? "", queryClient),
	);

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
		updateOrganizationMutation.error ?? deleteOrganizationMutation.error;

	const handleToggleWorkspaceSharing = async (enabled: boolean) => {
		if (!enabled) {
			setIsDisableDialogOpen(true);
		} else {
			try {
				await patchSharingSettingsMutation.mutateAsync({
					sharing_disabled: false,
				});
				displaySuccess("Workspace sharing enabled.");
			} catch (error) {
				displayError(
					getErrorMessage(error, "Failed to enable workspace sharing"),
				);
			}
		}
	};

	const handleConfirmDisableSharing = async () => {
		try {
			await patchSharingSettingsMutation.mutateAsync({
				sharing_disabled: true,
			});
			displaySuccess("Workspace sharing disabled.");
			setIsDisableDialogOpen(false);
		} catch (error) {
			displayError(
				getErrorMessage(error, "Failed to disable workspace sharing"),
			);
		}
	};

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
				workspaceSharingEnabled={
					!(sharingSettingsQuery.data?.sharing_disabled ?? false)
				}
				onToggleWorkspaceSharing={handleToggleWorkspaceSharing}
				isTogglingWorkspaceSharing={patchSharingSettingsMutation.isPending}
			/>

			<DisableWorkspaceSharingDialog
				isOpen={isDisableDialogOpen}
				organizationId={organization.id}
				onConfirm={handleConfirmDisableSharing}
				onCancel={() => setIsDisableDialogOpen(false)}
				isLoading={patchSharingSettingsMutation.isPending}
			/>
		</>
	);
};

export default OrganizationSettingsPage;
