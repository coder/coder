import { useState } from "react";
import type { AssignableRoles, SlimRole } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { getRoleNames } from "./index";
import { RoleSelector } from "./RoleSelector";

type RoleSelectorDialogProps = {
	/**
	 * The user who is currently being edited. The dialog will be hidden if no
	 * no user is provided.
	 */
	user?: ThingWithRoles;
	/** The roles available in this context that can be given or removed from the user */
	availableRoles?: AssignableRoles[];
	/**
	 * Extra roles to display as always-granted (implied) below `Member`.
	 * Pass through to the underlying selector. Used on the org members
	 * editor to surface the org's `default_org_member_roles`.
	 */
	additionalImpliedRoles?: AssignableRoles[];

	onCancel: () => void;
	onUpdateRoles: (roles: string[]) => Promise<void>;
	isUpdatingRoles: boolean;
};

type ThingWithRoles = {
	username: string;
	email: string;
	roles: readonly SlimRole[];
	avatar_url?: string;
};

export const RoleSelectorDialog: React.FC<RoleSelectorDialogProps> = ({
	user,
	availableRoles = [],
	additionalImpliedRoles = [],
	onCancel,
	onUpdateRoles,
	isUpdatingRoles,
}) => {
	if (!user) {
		return null;
	}

	return (
		<ActiveRoleSelectorDialog
			user={user}
			availableRoles={availableRoles}
			additionalImpliedRoles={additionalImpliedRoles}
			onCancel={onCancel}
			onUpdateRoles={onUpdateRoles}
			isUpdatingRoles={isUpdatingRoles}
		/>
	);
};

const ActiveRoleSelectorDialog: React.FC<Required<RoleSelectorDialogProps>> = ({
	user,
	availableRoles,
	additionalImpliedRoles,
	onCancel,
	onUpdateRoles,
	isUpdatingRoles,
}) => {
	const [selectedRoles, setSelectedRoles] = useState<Set<string>>(
		() => new Set(getRoleNames(user.roles)),
	);

	return (
		<Dialog
			open
			onOpenChange={(isOpen) => {
				if (!isOpen) {
					onCancel();
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<div className="flex flex-row justify-between items-center">
						<DialogTitle>Edit roles</DialogTitle>
						<AvatarData
							title={user.username}
							subtitle={user.email}
							src={user.avatar_url}
						/>
					</div>
				</DialogHeader>
				<RoleSelector
					hideLabel
					availableRoles={availableRoles}
					additionalImpliedRoles={additionalImpliedRoles}
					selectedRoles={selectedRoles}
					onChange={setSelectedRoles}
				/>
				<DialogFooter>
					<DialogActions
						onCancel={onCancel}
						onConfirm={() => onUpdateRoles([...selectedRoles])}
						confirmLoading={isUpdatingRoles}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
