import { type FC, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import { updateOrganization } from "#/api/queries/organizations";
import { deleteOrganizationRole, organizationRoles } from "#/api/queries/roles";
import type { Role } from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { useOrganizationSettings } from "#/modules/management/OrganizationSettingsLayout";
import { RequirePermission } from "#/modules/permissions/RequirePermission";
import { pageTitle } from "#/utils/page";
import { CustomRolesPageView } from "./CustomRolesPageView";

const CustomRolesPage: FC = () => {
	const queryClient = useQueryClient();
	const { custom_roles: isCustomRolesEnabled } = useFeatureVisibility();
	const { organization: organizationName } = useParams() as {
		organization: string;
	};
	const { organization, organizationPermissions } = useOrganizationSettings();
	const { experiments, entitlements } = useDashboard();
	const defaultRolesEnabled = experiments.includes("minimum-implicit-member");
	const defaultRolesEntitled =
		entitlements.features.multiple_organizations.enabled;

	const [roleToDelete, setRoleToDelete] = useState<Role>();

	const organizationRolesQuery = useQuery(organizationRoles(organizationName));
	const builtInRoles = organizationRolesQuery.data?.filter(
		(role) => role.built_in,
	);
	const customRoles = organizationRolesQuery.data?.filter(
		(role) => !role.built_in,
	);

	const deleteRoleMutation = useMutation(
		deleteOrganizationRole(queryClient, organizationName),
	);
	const updateOrganizationMutation = useMutation(
		updateOrganization(queryClient),
	);

	useEffect(() => {
		if (organizationRolesQuery.error) {
			toast.error(
				getErrorMessage(
					organizationRolesQuery.error,
					"Error loading custom roles.",
				),
				{
					description: getErrorDetail(organizationRolesQuery.error),
				},
			);
		}
	}, [organizationRolesQuery.error]);

	if (!organization) {
		return <EmptyState message="Organization not found" />;
	}

	return (
		<div className="w-full max-w-screen-2xl pb-10">
			<title>
				{pageTitle(
					"Custom Roles",
					organization.display_name || organization.name,
				)}
			</title>

			<RequirePermission
				isFeatureVisible={organizationPermissions?.viewOrgRoles ?? false}
			>
				<div className="flex flex-row gap-4 items-baseline justify-between">
					<SettingsHeader>
						<SettingsHeaderTitle>Roles</SettingsHeaderTitle>
						<SettingsHeaderDescription>
							Manage roles for this organization.
						</SettingsHeaderDescription>
					</SettingsHeader>
				</div>

				<CustomRolesPageView
					organization={organization}
					builtInRoles={builtInRoles}
					customRoles={customRoles}
					onDeleteRole={setRoleToDelete}
					canCreateOrgRole={organizationPermissions?.createOrgRoles ?? false}
					canUpdateOrgRole={organizationPermissions?.updateOrgRoles ?? false}
					canDeleteOrgRole={organizationPermissions?.deleteOrgRoles ?? false}
					canEditDefaultRoles={organizationPermissions?.editSettings ?? false}
					isCustomRolesEnabled={isCustomRolesEnabled}
					defaultRolesEnabled={defaultRolesEnabled}
					defaultRolesEntitled={defaultRolesEntitled}
					availableOrgRoles={organizationRolesQuery.data}
					isUpdatingDefaultRoles={updateOrganizationMutation.isPending}
					onUpdateDefaultRoles={async (roles) => {
						try {
							await updateOrganizationMutation.mutateAsync({
								organizationId: organization.id,
								req: { default_org_member_roles: roles },
							});
							toast.success("Default roles updated.");
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to update default roles."),
								{ description: getErrorDetail(error) },
							);
						}
					}}
				/>

				<DeleteDialog
					key={roleToDelete?.name}
					isOpen={roleToDelete !== undefined}
					confirmLoading={deleteRoleMutation.isPending}
					name={roleToDelete?.name ?? ""}
					entity="role"
					onCancel={() => setRoleToDelete(undefined)}
					onConfirm={async () => {
						try {
							if (roleToDelete) {
								await deleteRoleMutation.mutateAsync(roleToDelete.name, {
									onSuccess: () => {
										setRoleToDelete(undefined);
										organizationRolesQuery.refetch();
									},
								});
							}
							toast.success(
								roleToDelete
									? `Custom role "${roleToDelete.name}" deleted successfully.`
									: "Custom role deleted successfully.",
							);
						} catch (error) {
							toast.error(
								getErrorMessage(error, "Failed to delete custom role."),
								{
									description: getErrorDetail(error),
								},
							);
						}
					}}
				/>
			</RequirePermission>
		</div>
	);
};

export default CustomRolesPage;
