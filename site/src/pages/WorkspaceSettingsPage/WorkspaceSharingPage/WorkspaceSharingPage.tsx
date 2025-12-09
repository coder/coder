import { getErrorMessage } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import {
	setWorkspaceGroupRole,
	setWorkspaceUserRole,
	workspaceACL,
} from "api/queries/workspaces";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import { workspaceChecks } from "modules/workspaces/permissions";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const WorkspaceSharingPage: FC = () => {
	const workspace = useWorkspaceSettings();
	const queryClient = useQueryClient();

	const workspaceACLQuery = useQuery(workspaceACL(workspace.id));
	const checks = workspaceChecks(workspace);
	const permissionsQuery = useQuery({
		...checkAuthorization({ checks }),
	});
	const permissions = permissionsQuery.data as WorkspacePermissions | undefined;

	const addUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const updateUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const removeUserMutation = useMutation(setWorkspaceUserRole(queryClient));

	const addGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const updateGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const removeGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));

	const canUpdatePermissions = Boolean(permissions?.updateWorkspace);

	return (
		<div className="flex flex-col gap-12 max-w-screen-md">
			<title>{pageTitle(workspace.name, "Sharing")}</title>

			<header className="flex flex-col">
				<div className="flex flex-col gap-2">
					<h1 className="text-3xl m-0">Workspace sharing</h1>
					<p className="flex flex-row gap-1 text-sm text-content-secondary font-medium m-0">
						Workspace sharing allows you to share workspaces with other users
						and groups.
						{/* TODO: ADD DOCS LINK HERE <Link href={docs("/admin/users/idp-sync")}>View docs</Link> */}
					</p>
				</div>
			</header>

			{workspaceACLQuery.isError && (
				<ErrorAlert error={workspaceACLQuery.error} />
			)}
			{permissionsQuery.isError && (
				<ErrorAlert error={permissionsQuery.error} />
			)}

			<WorkspaceSharingPageView
				workspace={workspace}
				workspaceACL={workspaceACLQuery.data}
				canUpdatePermissions={canUpdatePermissions}
				onAddUser={async (user, role, reset) => {
					try {
						await addUserMutation.mutateAsync({
							workspaceId: workspace.id,
							userId: user.id,
							role,
						});
						displaySuccess("User added to workspace successfully!");
						reset();
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to add user to workspace"),
						);
					}
				}}
				isAddingUser={addUserMutation.isPending}
				onUpdateUser={async (user, role) => {
					try {
						await updateUserMutation.mutateAsync({
							workspaceId: workspace.id,
							userId: user.id,
							role,
						});
						displaySuccess("User role updated successfully!");
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to update user role in workspace"),
						);
					}
				}}
				updatingUserId={
					updateUserMutation.isPending
						? updateUserMutation.variables?.userId
						: undefined
				}
				onRemoveUser={async (user) => {
					try {
						await removeUserMutation.mutateAsync({
							workspaceId: workspace.id,
							userId: user.id,
							role: "",
						});
						displaySuccess("User removed successfully!");
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to remove user from workspace"),
						);
					}
				}}
				onAddGroup={async (group, role, reset) => {
					try {
						await addGroupMutation.mutateAsync({
							workspaceId: workspace.id,
							groupId: group.id,
							role,
						});
						displaySuccess("Group added to workspace successfully!");
						reset();
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to add group to workspace"),
						);
					}
				}}
				isAddingGroup={addGroupMutation.isPending}
				onUpdateGroup={async (group, role) => {
					try {
						await updateGroupMutation.mutateAsync({
							workspaceId: workspace.id,
							groupId: group.id,
							role,
						});
						displaySuccess("Group role updated successfully!");
					} catch (error) {
						displayError(
							getErrorMessage(
								error,
								"Failed to update group role in workspace",
							),
						);
					}
				}}
				updatingGroupId={
					updateGroupMutation.isPending
						? updateGroupMutation.variables?.groupId
						: undefined
				}
				onRemoveGroup={async (group) => {
					try {
						await removeGroupMutation.mutateAsync({
							workspaceId: workspace.id,
							groupId: group.id,
							role: "",
						});
						displaySuccess("Group removed successfully!");
					} catch (error) {
						displayError(
							getErrorMessage(error, "Failed to remove group from workspace"),
						);
					}
				}}
			/>
		</div>
	);
};

export default WorkspaceSharingPage;
