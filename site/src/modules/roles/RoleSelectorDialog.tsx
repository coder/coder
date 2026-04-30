import { useState } from "react";
import type { AssignableRoles, User } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
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
	availableRoles?: AssignableRoles[];

	onCancel: () => void;
	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const RoleSelectorDialog: React.FC<RoleSelectorDialogProps> = ({
	user,
	availableRoles,
	onCancel,
	onUpdateUserRoles,
	isUpdatingUserRoles,
}) => {
	if (!user) {
		return null;
	}

	return (
		<ActiveRoleSelectorDialog
			user={user}
			availableRoles={availableRoles}
			onCancel={onCancel}
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

	onCancel: () => void;
	onUpdateUserRoles: (userId: string, roles: string[]) => Promise<void>;
	isUpdatingUserRoles: boolean;
};

export const ActiveRoleSelectorDialog: React.FC<
	ActiveRoleSelectorDialogProps
> = ({
	user,
	availableRoles,
	onCancel,
	onUpdateUserRoles,
	isUpdatingUserRoles,
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
					selectedRoles={selectedRoles}
					onChange={setSelectedRoles}
				/>
				<DialogFooter>
					<DialogActionButtons
						onCancel={onCancel}
						onConfirm={() => onUpdateUserRoles(user.id, [...selectedRoles])}
						confirmLoading={isUpdatingUserRoles}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
