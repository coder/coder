import { useState } from "react";
import {
	type AssignableRoles,
	SlimRole,
	type User,
} from "#/api/typesGenerated";
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { DialogActionButtons } from "#/components/Dialogs/Dialog";
import { getRoleNames } from "./index";
import { RoleSelector } from "./RoleSelector";
import { AvatarData } from "#/components/Avatar/AvatarData";

type RoleSelectorDialogProps = {
	/**
	 * The user who is currently being edited. The dialog will be hidden if no
	 * no user is provided.
	 */
	user?: User;
	/** The roles available in this context that can be given or removed from the user */
	availableRoles?: AssignableRoles[];

	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const RoleSelectorDialog: React.FC<RoleSelectorDialogProps> = ({
	user,
	availableRoles,
	onUpdateUserRoles,
	isUpdatingUserRoles,
}) => {
	console.log("RoleSelectorDialog", user);

	if (!user) {
		return null;
	}

	return (
		<ActiveRoleSelectorDialog
			user={user}
			availableRoles={availableRoles}
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
	availableRoles?: AssignableRoles[];

	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const ActiveRoleSelectorDialog: React.FC<
	ActiveRoleSelectorDialogProps
> = ({ user, availableRoles, onUpdateUserRoles, isUpdatingUserRoles }) => {
	const [selectedRoles, setSelectedRoles] = useState<Set<string>>(
		() => new Set(getRoleNames(user.roles)),
	);

	return (
		<Dialog open>
			<DialogContent>
				<DialogHeader className="flex-row justify-between items-center">
					<DialogTitle>Edit roles</DialogTitle>
					<AvatarData
						title={user.username}
						subtitle={user.email}
						src={user.avatar_url}
					/>
				</DialogHeader>
				<RoleSelector
					availableRoles={availableRoles}
					selectedRoles={selectedRoles}
					onChange={setSelectedRoles}
				/>
				<DialogActionButtons />
			</DialogContent>
		</Dialog>
	);
};
