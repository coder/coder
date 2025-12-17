import type {
	Group,
	User,
	Workspace,
	WorkspaceACL,
	WorkspaceGroup,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
import { isGroup } from "modules/groups";
import {
	AddWorkspaceMemberForm,
	RoleSelectField,
	WorkspaceSharingForm,
} from "modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";
import { type FC, useState } from "react";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "./UserOrGroupAutocomplete";

type AddWorkspaceUserOrGroupProps = {
	organizationID: string;
	isLoading: boolean;
	workspaceACL: WorkspaceACL | undefined;
	onSubmit: (
		value: WorkspaceUser | Group | ({ role: WorkspaceRole } & User),
		role: WorkspaceRole,
		reset: () => void,
	) => void;
};

const AddWorkspaceUserOrGroup: FC<AddWorkspaceUserOrGroupProps> = ({
	organizationID,
	isLoading,
	workspaceACL,
	onSubmit,
}) => {
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);
	const [selectedRole, setSelectedRole] = useState<WorkspaceRole>("use");
	const excludeFromAutocomplete = workspaceACL
		? [...workspaceACL.group, ...workspaceACL.users]
		: [];

	const resetValues = () => {
		setSelectedOption(null);
		setSelectedRole("use");
	};

	return (
		<AddWorkspaceMemberForm
			isLoading={isLoading}
			disabled={!selectedRole || !selectedOption}
			onSubmit={() => {
				if (selectedOption && selectedRole) {
					onSubmit(
						{
							...selectedOption,
							role: selectedRole,
						},
						selectedRole,
						resetValues,
					);
				}
			}}
		>
			<UserOrGroupAutocomplete
				organizationId={organizationID}
				value={selectedOption}
				exclude={excludeFromAutocomplete}
				onChange={(newValue) => {
					setSelectedOption(newValue);
				}}
			/>

			<RoleSelectField
				value={selectedRole}
				onChange={setSelectedRole}
				disabled={isLoading}
			/>
		</AddWorkspaceMemberForm>
	);
};

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
