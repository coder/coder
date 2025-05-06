import { setGroupRole, setUserRole, templateACL } from "api/queries/templates";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Paywall } from "components/Paywall/Paywall";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import type { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplatePermissionsPageView } from "./TemplatePermissionsPageView";

const TemplatePermissionsPage: FC = () => {
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
			<Helmet>
				<title>{pageTitle(template.name, "Permissions")}</title>
			</Helmet>
			{!isTemplateRBACEnabled ? (
				<Paywall
					message="Template permissions"
					description="Control access of templates for users and groups to templates. You need an Premium license to use this feature."
					documentationLink={docs("/admin/templates/template-permissions")}
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
					isAddingUser={addUserMutation.isLoading}
					onUpdateUser={async (user, role) => {
						await updateUserMutation.mutateAsync({
							templateId: template.id,
							userId: user.id,
							role,
						});
						displaySuccess("User role updated successfully!");
					}}
					updatingUserId={
						updateUserMutation.isLoading
							? updateUserMutation.variables?.userId
							: undefined
					}
					onRemoveUser={async (user) => {
						await removeUserMutation.mutateAsync({
							templateId: template.id,
							userId: user.id,
							role: "",
						});
						displaySuccess("User removed successfully!");
					}}
					onAddGroup={async (group, role, reset) => {
						await addGroupMutation.mutateAsync({
							templateId: template.id,
							groupId: group.id,
							role,
						});
						reset();
					}}
					isAddingGroup={addGroupMutation.isLoading}
					onUpdateGroup={async (group, role) => {
						await updateGroupMutation.mutateAsync({
							templateId: template.id,
							groupId: group.id,
							role,
						});
						displaySuccess("Group role updated successfully!");
					}}
					updatingGroupId={
						updateGroupMutation.isLoading
							? updateGroupMutation.variables?.groupId
							: undefined
					}
					onRemoveGroup={async (group) => {
						await removeGroupMutation.mutateAsync({
							groupId: group.id,
							templateId: template.id,
							role: "",
						});
						displaySuccess("Group removed successfully!");
					}}
				/>
			)}
		</>
	);
};

export default TemplatePermissionsPage;
