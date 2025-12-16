import { checkAuthorization } from "api/queries/authCheck";
import {
	setWorkspaceGroupRole,
	setWorkspaceUserRole,
	workspaceACL,
} from "api/queries/workspaces";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { Link } from "components/Link/Link";
import type { WorkspacePermissions } from "modules/workspaces/permissions";
import { workspaceChecks } from "modules/workspaces/permissions";
import type { FC } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { docs } from "utils/docs";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "../WorkspaceSettingsLayout";
import { WorkspaceSharingPageView } from "./WorkspaceSharingPageView";

const WorkspaceSharingPage: FC = () => {
	const workspace = useWorkspaceSettings();
	const queryClient = useQueryClient();

	const workspaceACLQuery = useQuery(workspaceACL(workspace.id));
	const checks = workspaceChecks(workspace);
	const permissionsQuery = useQuery<WorkspacePermissions>({
		...checkAuthorization({ checks }),
	});
	const permissions = permissionsQuery.data;

	const addUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const updateUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const removeUserMutation = useMutation(setWorkspaceUserRole(queryClient));

	const addGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const updateGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const removeGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));

	const canUpdatePermissions = Boolean(permissions?.updateWorkspace);

	const mutationError =
		addUserMutation.error ??
		updateUserMutation.error ??
		removeUserMutation.error ??
		addGroupMutation.error ??
		updateGroupMutation.error ??
		removeGroupMutation.error;

	return (
		<div className="flex flex-col gap-12 max-w-screen-md">
			<title>{pageTitle(workspace.name, "Sharing")}</title>

			<header className="flex flex-col">
				<div className="flex flex-col gap-2">
					<h1 className="text-3xl m-0">Workspace sharing</h1>
					<p className="flex flex-row gap-1 text-sm text-content-secondary font-medium m-0">
						Workspace sharing allows you to share workspaces with other users
						and groups.{" "}
						<Link href={docs("/user-guides/shared-workspaces")}>View docs</Link>
					</p>
				</div>
			</header>

			{workspaceACLQuery.isError && (
				<ErrorAlert error={workspaceACLQuery.error} />
			)}
			{permissionsQuery.isError && (
				<ErrorAlert error={permissionsQuery.error} />
			)}
			{Boolean(mutationError) && <ErrorAlert error={mutationError} />}

			<WorkspaceSharingPageView
				workspace={workspace}
				workspaceACL={workspaceACLQuery.data}
				canUpdatePermissions={canUpdatePermissions}
				onAddUser={async (user, role, reset) => {
					await addUserMutation.mutateAsync({
						workspaceId: workspace.id,
						userId: user.id,
						role,
					});
					displaySuccess("User added to workspace successfully!");
					reset();
				}}
				isAddingUser={addUserMutation.isPending}
				onUpdateUser={async (user, role) => {
					await updateUserMutation.mutateAsync({
						workspaceId: workspace.id,
						userId: user.id,
						role,
					});
					displaySuccess("User role updated successfully!");
				}}
				updatingUserId={
					updateUserMutation.isPending
						? updateUserMutation.variables?.userId
						: undefined
				}
				onRemoveUser={async (user) => {
					await removeUserMutation.mutateAsync({
						workspaceId: workspace.id,
						userId: user.id,
						role: "",
					});
					displaySuccess("User removed successfully!");
				}}
				onAddGroup={async (group, role, reset) => {
					await addGroupMutation.mutateAsync({
						workspaceId: workspace.id,
						groupId: group.id,
						role,
					});
					displaySuccess("Group added to workspace successfully!");
					reset();
				}}
				isAddingGroup={addGroupMutation.isPending}
				onUpdateGroup={async (group, role) => {
					await updateGroupMutation.mutateAsync({
						workspaceId: workspace.id,
						groupId: group.id,
						role,
					});
					displaySuccess("Group role updated successfully!");
				}}
				updatingGroupId={
					updateGroupMutation.isPending
						? updateGroupMutation.variables?.groupId
						: undefined
				}
				onRemoveGroup={async (group) => {
					await removeGroupMutation.mutateAsync({
						workspaceId: workspace.id,
						groupId: group.id,
						role: "",
					});
					displaySuccess("Group removed successfully!");
				}}
			/>
		</div>
	);
};

export default WorkspaceSharingPage;
