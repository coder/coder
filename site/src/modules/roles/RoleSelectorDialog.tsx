import { useState } from "react";
import {
	type AssignableRoles,
	SlimRole,
	type User,
} from "#/api/typesGenerated";
import { Dialog, DialogContent } from "#/components/Dialog/Dialog";
import { DialogActionButtons } from "#/components/Dialogs/Dialog";
import { getRoleNames } from "./index";
import { RoleSelector } from "./RoleSelector";

type RoleSelectorDialogProps = {
	/**
	 * The user who is currently being edited. The dialog will be hidden if no
	 * no user is provided.
	 */
	user?: User;
	/** The roles available in this context that can be given or removed from the user */
	assignableRoles?: AssignableRoles[];

	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const RoleSelectorDialog: React.FC<RoleSelectorDialogProps> = ({
	user,
	assignableRoles,
	onUpdateUserRoles,
	isUpdatingUserRoles,
}) => {
	if (!user) {
		return null;
	}

	return (
		<ActiveRoleSelectorDialog
			user={user}
			assignableRoles={assignableRoles}
			onUpdateUserRoles={onUpdateUserRoles}
			isUpdatingUserRoles={isUpdatingUserRoles}
		/>
	);
};

type ActiveRoleSelectorDialogProps = {
	/**
	 * The user who is currently being edited.
	 */
	user: User;
	/** The roles available in this context that can be given or removed from the user */
	assignableRoles: AssignableRoles[];

	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const ActiveRoleSelectorDialog: React.FC<
	ActiveRoleSelectorDialogProps
> = ({ user, assignableRoles, onUpdateUserRoles, isUpdatingUserRoles }) => {
	const [selectedRoles, setSelectedRoles] = useState<Set<string>>(
		() => new Set(getRoleNames(user.roles)),
	);

	return (
		<Dialog>
			<DialogContent>
				<RoleSelector
					roles={assignableRoles}
					selectedRoles={selectedRoles}
					onChange={setSelectedRoles}
				/>
				<DialogActionButtons />
			</DialogContent>
		</Dialog>
	);
};
