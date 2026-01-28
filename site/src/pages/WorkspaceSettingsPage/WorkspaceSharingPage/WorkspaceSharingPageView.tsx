import type {
	Group,
	Workspace,
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
import { isGroup } from "modules/groups";
import { AddWorkspaceUserOrGroup } from "modules/workspaces/WorkspaceSharingForm/AddWorkspaceUserOrGroup";
import { WorkspaceSharingForm } from "modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";
import type { FC } from "react";

interface WorkspaceSharingPageViewProps {
	workspace: Workspace;
	workspaceACL: WorkspaceACL | undefined;
	canUpdatePermissions: boolean;
	error: unknown;
	onAddUser: (
		user: WorkspaceUser,
		role: WorkspaceRole,
		reset: () => void,
	) => void;
	isAddingUser: boolean;
	onUpdateUser: (user: WorkspaceUser, role: WorkspaceRole) => void;
	updatingUserId: WorkspaceUser["id"] | undefined;
	onRemoveUser: (user: WorkspaceUser) => void;
	onAddGroup: (group: Group, role: WorkspaceRole, reset: () => void) => void;
	isAddingGroup: boolean;
	onUpdateGroup: (group: WorkspaceGroup, role: WorkspaceRole) => void;
	updatingGroupId?: WorkspaceGroup["id"] | undefined;
	onRemoveGroup: (group: Group) => void;
	hasRemovedMember?: boolean;
}

export const WorkspaceSharingPageView: FC<WorkspaceSharingPageViewProps> = ({
	workspace,
	workspaceACL,
	canUpdatePermissions,
	error,
	onAddUser,
	isAddingUser,
	updatingUserId,
	onUpdateUser,
	onRemoveUser,
	onAddGroup,
	isAddingGroup,
	updatingGroupId,
	onUpdateGroup,
	onRemoveGroup,
	hasRemovedMember,
}) => {
	return (
		<WorkspaceSharingForm
			workspaceACL={workspaceACL}
			canUpdatePermissions={canUpdatePermissions}
			error={error}
			updatingUserId={updatingUserId}
			onUpdateUser={onUpdateUser}
			onRemoveUser={onRemoveUser}
			updatingGroupId={updatingGroupId}
			onUpdateGroup={onUpdateGroup}
			onRemoveGroup={onRemoveGroup}
			showRestartWarning={hasRemovedMember}
			addMemberForm={
				<AddWorkspaceUserOrGroup
					organizationID={workspace.organization_id}
					workspaceACL={workspaceACL}
					isLoading={isAddingUser || isAddingGroup}
					onSubmit={(value, role, resetAutocomplete) =>
						isGroup(value)
							? onAddGroup(value, role, resetAutocomplete)
							: onAddUser(value, role, resetAutocomplete)
					}
				/>
			}
		/>
	);
};
