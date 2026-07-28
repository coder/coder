import type { FC } from "react";
import { useState } from "react";
import type { AssignableRoles } from "#/api/typesGenerated";
import {
	Dialog,
	DialogActions,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { RoleSelector } from "#/modules/roles/RoleSelector";

interface DefaultRolesDialogProps {
	open: boolean;
	currentRoles: readonly string[];
	availableRoles?: AssignableRoles[];
	onCancel: () => void;
	onConfirm: (roles: string[]) => Promise<void>;
	isUpdating: boolean;
}

export const DefaultRolesDialog: FC<DefaultRolesDialogProps> = ({
	open,
	currentRoles,
	availableRoles,
	onCancel,
	onConfirm,
	isUpdating,
}) => {
	if (!open) {
		return null;
	}

	return (
		<ActiveDefaultRolesDialog
			currentRoles={currentRoles}
			availableRoles={availableRoles ?? []}
			onCancel={onCancel}
			onConfirm={onConfirm}
			isUpdating={isUpdating}
		/>
	);
};

interface ActiveProps {
	currentRoles: readonly string[];
	availableRoles: AssignableRoles[];
	onCancel: () => void;
	onConfirm: (roles: string[]) => Promise<void>;
	isUpdating: boolean;
}

const ActiveDefaultRolesDialog: FC<ActiveProps> = ({
	currentRoles,
	availableRoles,
	onCancel,
	onConfirm,
	isUpdating,
}) => {
	const [selected, setSelected] = useState<Set<string>>(
		() => new Set(currentRoles),
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
					<DialogTitle>Edit default roles</DialogTitle>
					<DialogDescription>
						These roles are attached to every member of this organization. Use
						an empty selection to grant new members only the floor.
					</DialogDescription>
				</DialogHeader>
				<RoleSelector
					hideLabel
					availableRoles={availableRoles}
					selectedRoles={selected}
					onChange={setSelected}
					disabledReason={(role) =>
						role.built_in
							? undefined
							: "Only built-in roles are supported as organization default roles"
					}
				/>
				<DialogFooter>
					<DialogActions
						onCancel={onCancel}
						onConfirm={() => onConfirm([...selected])}
						confirmLoading={isUpdating}
					/>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
