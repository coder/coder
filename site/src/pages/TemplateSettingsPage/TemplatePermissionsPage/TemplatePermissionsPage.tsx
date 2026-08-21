import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";
import { getErrorDetail, getErrorMessage } from "#/api/errors";
import {
	setGroupRole,
	setUserRole,
	templateACL,
} from "#/api/queries/templates";
import {
	SettingsHeader,
	SettingsHeaderDescription,
	SettingsHeaderDocsLink,
	SettingsHeaderTitle,
} from "#/components/SettingsHeader/SettingsHeader";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useFeatureVisibility } from "#/modules/dashboard/useFeatureVisibility";
import { PremiumPaywall } from "#/modules/paywall/PremiumPaywall";
import { docs } from "#/utils/docs";
import { pageTitle } from "#/utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";

const TemplatePermissionsPage: FC = () => {
	const { permissions: authPermissions } = useAuthenticated();
	const { template, permissions } = useTemplateSettings();
	const { template_rbac: isTemplateRBACEnabled } = useFeatureVisibility();
	const templateACLQuery = useQuery(templateACL(template.id));
	const queryClient = useQueryClient();

	const addUserMutation = useMutation(setUserRole(queryClient));
	const updateUserMutation = useMutation(setUserRole(queryClient));
	const removeUserMutation = useMutation(setUserRole(queryClient));

	const addGroupMutation = useMutation(setGroupRole(queryClient));
	const updateGroupMutation = useMutation(setGroupRole(queryClient));
	const removeGroupMutation = useMutation(setGroupRole(queryClient));

	return (
		<>
			<title>{pageTitle(template.name, "Permissions")}</title>

			<div className="flex flex-col gap-12">
				<SettingsHeader
					actions={
						<SettingsHeaderDocsLink
							href={docs("/admin/templates/template-permissions")}
						/>
					}
				>
					<SettingsHeaderTitle>Permissions</SettingsHeaderTitle>
					<SettingsHeaderDescription>
						Manage which members and groups can use this template.
					</SettingsHeaderDescription>
				</SettingsHeader>

				{!isTemplateRBACEnabled ? (
					<PremiumPaywall
						source="template_permissions"
						message="Template permissions"
						description="Restrict template access by user or group."
						features={[
							"Choose Use or Admin-level access",
							"Prevent unauthorized template use",
						]}
						canViewPremium={authPermissions.viewAllLicenses}
					/>
				) : (
					<TemplatePermissionsPageView
						templateID={template.id}
						templateACL={templateACLQuery.data}
						canUpdatePermissions={Boolean(permissions?.canUpdateTemplate)}
						onAddUser={async (user, role, reset) => {
							await addUserMutation.mutateAsync({
								templateId: template.id,
								userId: user.id,
								role,
							});
							reset();
						}}
						isAddingUser={addUserMutation.isPending}
						onUpdateUser={async (user, role) => {
							await updateUserMutation.mutateAsync(
								{
									templateId: template.id,
									userId: user.id,
									role,
								},
								{
									onSuccess: () => {
										toast.success(
											`Role for "${user.username}" updated to "${role}" successfully.`,
										);
									},
									onError: (error) => {
										toast.error(
											getErrorMessage(
												error,
												`Failed to update role for "${user.username}".`,
											),
											{
												description: getErrorDetail(error),
											},
										);
									},
								},
							);
						}}
						updatingUserId={
							updateUserMutation.isPending
								? updateUserMutation.variables?.userId
								: undefined
						}
						onRemoveUser={async (user) => {
							await removeUserMutation.mutateAsync(
								{
									templateId: template.id,
									userId: user.id,
									role: "",
								},
								{
									onSuccess: () => {
										toast.success(
											`User "${user.username}" removed successfully.`,
										);
									},
									onError: (error) => {
										toast.error(
											getErrorMessage(
												error,
												`Failed to remove user "${user.username}".`,
											),
											{
												description: getErrorDetail(error),
											},
										);
									},
								},
							);
						}}
						onAddGroup={async (group, role, reset) => {
							await addGroupMutation.mutateAsync({
								templateId: template.id,
								groupId: group.id,
								role,
							});
							reset();
						}}
						isAddingGroup={addGroupMutation.isPending}
						onUpdateGroup={async (group, role) => {
							await updateGroupMutation.mutateAsync(
								{
									templateId: template.id,
									groupId: group.id,
									role,
								},
								{
									onSuccess: () => {
										toast.success(
											`Role for "${group.display_name || group.name}" updated to "${role}" successfully.`,
										);
									},
									onError: (error) => {
										toast.error(
											getErrorMessage(
												error,
												`Failed to update role for "${group.display_name || group.name}".`,
											),
											{
												description: getErrorDetail(error),
											},
										);
									},
								},
							);
						}}
						updatingGroupId={
							updateGroupMutation.isPending
								? updateGroupMutation.variables?.groupId
								: undefined
						}
						onRemoveGroup={async (group) => {
							await removeGroupMutation.mutateAsync(
								{
									groupId: group.id,
									templateId: template.id,
									role: "",
								},
								{
									onSuccess: () => {
										toast.success(
											`Group "${group.display_name || group.name}" removed successfully.`,
										);
									},
									onError: (error) => {
										toast.error(
											getErrorMessage(
												error,
												`Failed to remove group "${group.display_name || group.name}".`,
											),
											{
												description: getErrorDetail(error),
											},
										);
									},
								},
							);
						}}
					/>
				)}
			</div>
		</>
	);
};

export default TemplatePermissionsPage;
