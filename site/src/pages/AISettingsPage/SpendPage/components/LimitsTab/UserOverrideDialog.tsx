import type { FC } from "react";
import { getErrorMessage } from "#/api/errors";
import type { User } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "#/components/Dialog/Dialog";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import { UserAutocomplete } from "#/components/UserAutocomplete/UserAutocomplete";
import { isPositiveFiniteDollarAmount } from "#/utils/currency";
import type { UserOverrideUser } from "./UserOverridesSection";

interface UserOverrideDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	selectedUser: User | null;
	onSelectedUserChange: (user: User | null) => void;
	userOverrideAmount: string;
	onUserOverrideAmountChange: (amount: string) => void;
	selectedUserAlreadyOverridden: boolean;
	editingUserOverride: UserOverrideUser | null;
	upsertPending: boolean;
	upsertError: Error | null;
	onSave: () => void;
	amountInputId: string;
}

export const UserOverrideDialog: FC<UserOverrideDialogProps> = ({
	open,
	onOpenChange,
	selectedUser,
	onSelectedUserChange,
	userOverrideAmount,
	onUserOverrideAmountChange,
	selectedUserAlreadyOverridden,
	editingUserOverride,
	upsertPending,
	upsertError,
	onSave,
	amountInputId,
}) => {
	const isEditing = editingUserOverride !== null;
	const saveDisabled = isEditing
		? upsertPending || !isPositiveFiniteDollarAmount(userOverrideAmount)
		: upsertPending ||
			!selectedUser ||
			selectedUserAlreadyOverridden ||
			!isPositiveFiniteDollarAmount(userOverrideAmount);

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (upsertPending) {
					return;
				}

				onOpenChange(nextOpen);
				if (!nextOpen) {
					onSelectedUserChange(null);
					onUserOverrideAmountChange("");
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{isEditing ? "Update user budget" : "Add user budget"}
					</DialogTitle>
					<DialogDescription>
						{isEditing
							? "Update this user's spend limit override."
							: "Set a spend limit override for a specific user."}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-5">
					<div className="space-y-1.5">
						{editingUserOverride ? (
							<>
								<Label>User</Label>
								<div className="rounded-md border border-border bg-surface-secondary/40 p-2">
									<AvatarData
										title={
											editingUserOverride.name || editingUserOverride.username
										}
										subtitle={`@${editingUserOverride.username}`}
										src={editingUserOverride.avatar_url}
										imgFallbackText={editingUserOverride.username}
									/>
								</div>
							</>
						) : (
							<UserAutocomplete
								value={selectedUser}
								onChange={onSelectedUserChange}
								label="User"
							/>
						)}
					</div>
					<div className="space-y-1.5">
						<Label htmlFor={amountInputId}>Spend limit ($)</Label>
						<Input
							id={amountInputId}
							type="number"
							step="0.01"
							min="0.01"
							disabled={upsertPending}
							value={userOverrideAmount}
							onChange={(event) =>
								onUserOverrideAmountChange(event.target.value)
							}
							placeholder="0.00"
						/>
					</div>
					{!isEditing && selectedUserAlreadyOverridden && (
						<p className="text-xs text-content-warning">
							This user already has an override.
						</p>
					)}
					{upsertError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(upsertError, "Failed to save the override.")}
						</p>
					)}
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						type="button"
						onClick={() => {
							onOpenChange(false);
							onSelectedUserChange(null);
							onUserOverrideAmountChange("");
						}}
						disabled={upsertPending}
					>
						Cancel
					</Button>
					<Button type="button" onClick={onSave} disabled={saveDisabled}>
						{upsertPending ? <Spinner loading className="size-4" /> : null}
						{isEditing ? "Update budget" : "Add user"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
