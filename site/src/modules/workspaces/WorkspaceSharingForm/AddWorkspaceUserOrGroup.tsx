import type {
	Group,
	User,
	WorkspaceACL,
	WorkspaceRole,
	WorkspaceUser,
} from "api/typesGenerated";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";
import {
	AddWorkspaceMemberForm,
	RoleSelectField,
} from "modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";
import { type FC, useState } from "react";

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

export const AddWorkspaceUserOrGroup: FC<AddWorkspaceUserOrGroupProps> = ({
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
