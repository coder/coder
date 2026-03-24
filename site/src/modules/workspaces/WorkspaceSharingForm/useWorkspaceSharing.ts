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
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { toast } from "sonner";

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
		const mutation = addUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role,
		});
		toast.promise(mutation, {
			loading: `Adding ${user.username} to workspace...`,
			success: `"${user.username}" added to workspace successfully.`,
		});
		reset();
	};

	const updateUser = async (user: WorkspaceUser, role: WorkspaceRole) => {
		await updateUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role,
		});
		toast.success(`"${user.username}" role updated successfully.`);
	};

	const removeUser = async (user: WorkspaceUser) => {
		await removeUserMutation.mutateAsync({
			workspaceId: workspace.id,
			userId: user.id,
			role: "",
		});
		setHasRemovedMember(true);
		toast.success(`"${user.username}" removed successfully.`);
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
		toast.success(`Group "${group.name}" added to workspace successfully.`);
		reset();
	};

	const updateGroup = async (group: WorkspaceGroup, role: WorkspaceRole) => {
		await updateGroupMutation.mutateAsync({
			workspaceId: workspace.id,
			groupId: group.id,
			role,
		});
		toast.success(`Group role "${role}" updated successfully.`);
	};

	const removeGroup = async (group: Group) => {
		await removeGroupMutation.mutateAsync({
			workspaceId: workspace.id,
			groupId: group.id,
			role: "",
		});
		setHasRemovedMember(true);
		toast.success(`Group "${group.name}" removed successfully.`);
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
