import { type FC, useId } from "react";
import { formatCostMicros, isPositiveFiniteDollarAmount } from "utils/currency";
import { getErrorMessage } from "#/api/errors";
import type { User } from "#/api/typesGenerated";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { UserAutocomplete } from "#/components/UserAutocomplete/UserAutocomplete";
import { SectionHeader } from "../SectionHeader";

interface UserOverridesSectionProps {
	overrides: ReadonlyArray<{
		user_id: string;
		name: string;
		username: string;
		avatar_url: string;
		spend_limit_micros: number | null;
	}>;
	showUserForm: boolean;
	onShowUserFormChange: (show: boolean) => void;
	selectedUser: User | null;
	onSelectedUserChange: (user: User | null) => void;
	userOverrideAmount: string;
	onUserOverrideAmountChange: (amount: string) => void;
	selectedUserAlreadyOverridden: boolean;
	editingUserOverride: {
		user_id: string;
		name: string;
		username: string;
		avatar_url: string;
	} | null;
	onEditUserOverride: (
		override: UserOverridesSectionProps["overrides"][number],
	) => void;
	onAddOverride: () => void;
	onDeleteOverride: (userID: string) => void;
	upsertPending: boolean;
	upsertError: Error | null;
	deletePending: boolean;
	deleteError: Error | null;
}

export const UserOverridesSection: FC<UserOverridesSectionProps> = ({
	overrides,
	showUserForm,
	onShowUserFormChange,
	selectedUser,
	onSelectedUserChange,
	userOverrideAmount,
	onUserOverrideAmountChange,
	selectedUserAlreadyOverridden,
	editingUserOverride,
	onEditUserOverride,
	onAddOverride,
	onDeleteOverride,
	upsertPending,
	upsertError,
	deletePending,
	deleteError,
}) => {
	const userOverrideAmountId = useId();
	const isEditing = editingUserOverride !== null;

	return (
		<section className="space-y-4">
			<SectionHeader
				label="Per-User Overrides"
				description="Override the deployment default spend limit for specific users. User overrides take highest priority, followed by group limits, then the deployment default."
			/>

			<div className="space-y-4">
				{overrides.length > 0 ? (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>User</TableHead>
								<TableHead>Spend Limit</TableHead>
								<TableHead className="w-[160px]">Actions</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{overrides.map((override) => (
								<TableRow key={override.user_id}>
									<TableCell>
										<AvatarData
											title={override.name || override.username}
											subtitle={`@${override.username}`}
											src={override.avatar_url}
											imgFallbackText={override.username}
										/>
									</TableCell>
									<TableCell>
										{override.spend_limit_micros !== null
											? formatCostMicros(override.spend_limit_micros)
											: "Unlimited"}
									</TableCell>
									<TableCell>
										<div className="flex gap-2">
											<Button
												variant="outline"
												size="sm"
												type="button"
												onClick={() => onEditUserOverride(override)}
												disabled={deletePending || upsertPending}
											>
												Edit
											</Button>
											<Button
												variant="outline"
												size="sm"
												type="button"
												onClick={() => void onDeleteOverride(override.user_id)}
												disabled={deletePending || upsertPending || isEditing}
											>
												Delete
											</Button>
										</div>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				) : (
					<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
						No overrides configured.
					</div>
				)}

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete override.")}
					</p>
				)}

				{!showUserForm ? (
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => onShowUserFormChange(true)}
						disabled={isEditing}
					>
						Add User
					</Button>
				) : (
					<div className="space-y-3 rounded-lg border border-border bg-surface-secondary/40 p-4">
						<div className="flex flex-col gap-3 md:flex-row md:items-end">
							<div className="flex-1 space-y-1">
								{editingUserOverride ? (
									<>
										<Label>User</Label>
										<div className="rounded-md border border-border bg-surface-primary p-2">
											<AvatarData
												title={
													editingUserOverride.name ||
													editingUserOverride.username
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
							<div className="flex-1 space-y-1">
								<Label htmlFor={userOverrideAmountId}>Spend Limit ($)</Label>
								<Input
									id={userOverrideAmountId}
									type="number"
									step="0.01"
									min="0.01"
									disabled={upsertPending}
									className="h-9 min-w-0 text-[13px]"
									value={userOverrideAmount}
									onChange={(event) =>
										onUserOverrideAmountChange(event.target.value)
									}
									placeholder="0.00"
								/>
							</div>
							<div className="flex gap-2 md:pb-0.5">
								<Button
									size="sm"
									type="button"
									onClick={() => void onAddOverride()}
									disabled={
										isEditing
											? upsertPending ||
												!isPositiveFiniteDollarAmount(userOverrideAmount)
											: upsertPending ||
												!selectedUser ||
												selectedUserAlreadyOverridden ||
												!isPositiveFiniteDollarAmount(userOverrideAmount)
									}
								>
									{upsertPending ? (
										<Spinner loading className="h-4 w-4" />
									) : null}
									{isEditing ? "Save" : "Add"}
								</Button>
								<Button
									variant="outline"
									size="sm"
									type="button"
									onClick={() => {
										onShowUserFormChange(false);
										onSelectedUserChange(null);
										onUserOverrideAmountChange("");
									}}
									disabled={upsertPending}
								>
									Cancel
								</Button>
							</div>
						</div>
					</div>
				)}
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
		</section>
	);
};
