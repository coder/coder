import { useId, useRef, useState } from "react";
import type {
	Group,
	User,
	WorkspaceACL,
	WorkspaceRole,
	WorkspaceUser,
} from "#/api/typesGenerated";
import { Label } from "#/components/Label/Label";
import {
	UserOrGroupAutocomplete,
	type UserOrGroupAutocompleteValue,
} from "#/modules/workspaces/WorkspaceSharingForm/UserOrGroupAutocomplete";
import {
	AddWorkspaceMemberForm,
	RoleSelectField,
} from "#/modules/workspaces/WorkspaceSharingForm/WorkspaceSharingForm";

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

export const AddWorkspaceUserOrGroup: React.FC<
	AddWorkspaceUserOrGroupProps
> = ({ organizationID, isLoading, workspaceACL, onSubmit }) => {
	const [selectedOption, setSelectedOption] =
		useState<UserOrGroupAutocompleteValue>(null);
	const [selectedRole, setSelectedRole] = useState<WorkspaceRole>("use");
	const autocompleteTriggerRef = useRef<HTMLButtonElement>(null);
	const autocompleteId = useId();
	const roleId = useId();
	const autocompleteLabelId = `${autocompleteId}-label`;
	const excludeFromAutocomplete = workspaceACL
		? [...workspaceACL.group, ...workspaceACL.users]
		: [];

	const resetValues = () => {
		// Focus moves before the reset disables the submit button.
		autocompleteTriggerRef.current?.focus();
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
			<div className="flex flex-col gap-2">
				<Label id={autocompleteLabelId}>User or group</Label>
				<UserOrGroupAutocomplete
					id={autocompleteId}
					triggerAriaLabelledBy={`${autocompleteLabelId} ${autocompleteId}`}
					organizationId={organizationID}
					value={selectedOption}
					exclude={excludeFromAutocomplete}
					triggerRef={autocompleteTriggerRef}
					onChange={(newValue) => {
						setSelectedOption(newValue);
					}}
				/>
			</div>

			<div className="flex flex-col gap-2">
				<Label htmlFor={roleId}>Role</Label>
				<RoleSelectField
					id={roleId}
					value={selectedRole}
					onChange={setSelectedRole}
					disabled={isLoading}
				/>
			</div>
		</AddWorkspaceMemberForm>
	);
};
