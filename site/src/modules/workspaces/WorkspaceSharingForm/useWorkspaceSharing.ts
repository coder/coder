import {
	setWorkspaceGroupRole,
	setWorkspaceUserRole,
	workspaceACL,
} from "api/queries/workspaces";
import type {
	Group,
	Workspace,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";

/**
 * Encapsulates all data fetching and mutations for workspace sharing.
 * This hook manages the workspace ACL query and provides methods to
 * add, update, and remove users and groups from the workspace.
 */
export function useWorkspaceSharing(workspace: Workspace) {
	const queryClient = useQueryClient();
	const [hasRemovedMember, setHasRemovedMember] = useState(false);

	const workspaceACLQuery = useQuery(workspaceACL(workspace.id));

	const addUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const updateUserMutation = useMutation(setWorkspaceUserRole(queryClient));
	const removeUserMutation = useMutation(setWorkspaceUserRole(queryClient));

	const addGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const updateGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));
	const removeGroupMutation = useMutation(setWorkspaceGroupRole(queryClient));

	const addUser = async (
		user: WorkspaceUser,
		role: WorkspaceRole,
		reset: () => void,
	) => {
		await addUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role,
		});
		setHasRemovedMember(false);
		displaySuccess("User added to workspace successfully!");
		reset();
	};

	const updateUser = async (user: WorkspaceUser, role: WorkspaceRole) => {
		await updateUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role,
		});
		displaySuccess("User role updated successfully!");
	};

	const removeUser = async (user: WorkspaceUser) => {
		await removeUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role: "",
		});
		setHasRemovedMember(true);
		displaySuccess("User removed successfully!");
	};

	const addGroup = async (
		group: Group,
		role: WorkspaceRole,
		reset: () => void,
	) => {
		await addGroupMutation.mutateAsync({
			workspaceId: workspace.id,
			groupId: group.id,
			role,
		});
		setHasRemovedMember(false);
		displaySuccess("Group added to workspace successfully!");
		reset();
	};

	const updateGroup = async (group: WorkspaceGroup, role: WorkspaceRole) => {
		await updateGroupMutation.mutateAsync({
			workspaceId: workspace.id,
			groupId: group.id,
			role,
		});
		displaySuccess("Group role updated successfully!");
	};

	const removeGroup = async (group: Group) => {
		await removeGroupMutation.mutateAsync({
			workspaceId: workspace.id,
			groupId: group.id,
			role: "",
		});
		setHasRemovedMember(true);
		displaySuccess("Group removed successfully!");
	};

	const mutationError =
		addUserMutation.error ??
		updateUserMutation.error ??
		removeUserMutation.error ??
		addGroupMutation.error ??
		updateGroupMutation.error ??
		removeGroupMutation.error;

	return {
		workspaceACL: workspaceACLQuery.data,
		isLoading: workspaceACLQuery.isLoading,
		error: workspaceACLQuery.error,
		mutationError,
		hasRemovedMember,
		// User actions
		addUser,
		updateUser,
		removeUser,
		isAddingUser: addUserMutation.isPending,
		updatingUserId: updateUserMutation.isPending
			? updateUserMutation.variables?.userId
			: undefined,
		// Group actions
		addGroup,
		updateGroup,
		removeGroup,
		isAddingGroup: addGroupMutation.isPending,
		updatingGroupId: updateGroupMutation.isPending
			? updateGroupMutation.variables?.groupId
			: undefined,
	} as const;
}
