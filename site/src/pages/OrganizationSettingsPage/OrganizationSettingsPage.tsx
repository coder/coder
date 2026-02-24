import { getErrorDetail, getErrorMessage } from "api/errors";
import {
	deleteOrganization,
	patchWorkspaceSharingSettings,
	updateOrganization,
	workspaceSharingSettings,
} from "api/queries/organizations";
import { EmptyState } from "components/EmptyState/EmptyState";
import { useOrganizationSettings } from "modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "modules/permissions/RequirePermission";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate } from "react-router";
import { toast } from "sonner";
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
		const mutation = patchSharingSettingsMutation.mutateAsync({
			sharing_disabled: !enabled,
		});
		toast.promise(mutation, {
			loading: "Toggling workspace sharing...",
			success: enabled
				? "Workspace sharing enabled."
				: "Workspace sharing disabled.",
			error: (error) => ({
				message: enabled
					? "Failed to enable workspace sharing."
					: "Failed to disable workspace sharing.",
				description: getErrorDetail(error),
			}),
		});
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
					toast.success(
						`Organization "${updatedOrganization.name}" settings updated successfully.`,
					);
				}}
				onDeleteOrganization={async () => {
					try {
						await deleteOrganizationMutation.mutateAsync(organization.id);
						toast.success(
							`Organization "${organization.display_name || organization.name}" deleted successfully.`,
						);
						navigate("/organizations");
					} catch (error) {
						toast.error(
							getErrorMessage(
								error,
								`Failed to delete organization "${organization.name}".`,
							),
							{
								description: getErrorDetail(error),
							},
						);
					}
				}}
				workspaceSharingEnabled={
					!(sharingSettingsQuery.data?.sharing_disabled ?? false)
				}
				onToggleWorkspaceSharing={handleToggleWorkspaceSharing}
				isTogglingWorkspaceSharing={patchSharingSettingsMutation.isPending}
			/>
		</>
	);
};

export default OrganizationSettingsPage;
