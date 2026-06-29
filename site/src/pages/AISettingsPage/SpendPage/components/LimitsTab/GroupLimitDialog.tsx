import { CheckIcon } from "lucide-react";
import type { FC } from "react";
import { getErrorMessage } from "#/api/errors";
import type { Group } from "#/api/typesGenerated";
import { Autocomplete } from "#/components/Autocomplete/Autocomplete";
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
import { getGroupSubtitle } from "#/modules/groups";
import { isPositiveFiniteDollarAmount } from "#/utils/currency";
import type { GroupLimitOverrideGroup } from "./GroupLimitsSection";

interface GroupLimitDialogProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	selectedGroup: Group | null;
	onSelectedGroupChange: (group: Group | null) => void;
	groupAmount: string;
	onGroupAmountChange: (amount: string) => void;
	availableGroups: Group[];
	groupAutocompleteNoOptionsText: string;
	groupsLoading: boolean;
	editingGroupOverride: GroupLimitOverrideGroup | null;
	upsertPending: boolean;
	upsertError: Error | null;
	onSave: () => void;
	groupAutocompleteId: string;
	groupAmountId: string;
}

export const GroupLimitDialog: FC<GroupLimitDialogProps> = ({
	open,
	onOpenChange,
	selectedGroup,
	onSelectedGroupChange,
	groupAmount,
	onGroupAmountChange,
	availableGroups,
	groupAutocompleteNoOptionsText,
	groupsLoading,
	editingGroupOverride,
	upsertPending,
	upsertError,
	onSave,
	groupAutocompleteId,
	groupAmountId,
}) => {
	const isEditing = editingGroupOverride !== null;
	const saveDisabled = isEditing
		? upsertPending || !isPositiveFiniteDollarAmount(groupAmount)
		: upsertPending ||
			selectedGroup === null ||
			!isPositiveFiniteDollarAmount(groupAmount);

	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (upsertPending) {
					return;
				}

				onOpenChange(nextOpen);
				if (!nextOpen) {
					onSelectedGroupChange(null);
					onGroupAmountChange("");
				}
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>
						{isEditing ? "Update group budget" : "Add group budget"}
					</DialogTitle>
					<DialogDescription>
						{isEditing
							? "Update this group's spend limit override."
							: "Set a spend limit override for a specific group."}
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-5">
					<div className="space-y-1.5">
						{editingGroupOverride ? (
							<>
								<Label>Group</Label>
								<div className="rounded-md border border-border bg-surface-secondary/40 p-2">
									<AvatarData
										title={
											editingGroupOverride.group_display_name ||
											editingGroupOverride.group_name
										}
										subtitle={editingGroupOverride.group_name}
										src={editingGroupOverride.group_avatar_url}
										imgFallbackText={editingGroupOverride.group_name}
									/>
								</div>
							</>
						) : (
							<>
								<Label htmlFor={groupAutocompleteId}>Group</Label>
								<Autocomplete
									id={groupAutocompleteId}
									value={selectedGroup}
									onChange={onSelectedGroupChange}
									options={availableGroups}
									getOptionValue={(group) => group.id}
									getOptionLabel={(group) => group.display_name || group.name}
									isOptionEqualToValue={(option, optionValue) =>
										option.id === optionValue.id
									}
									renderOption={(option, isSelected) => (
										<div className="flex w-full items-center justify-between gap-2">
											<AvatarData
												title={option.display_name || option.name}
												subtitle={getGroupSubtitle(option)}
												src={option.avatar_url}
												imgFallbackText={option.name}
											/>
											{isSelected && <CheckIcon className="size-4 shrink-0" />}
										</div>
									)}
									placeholder="Search groups..."
									noOptionsText={groupAutocompleteNoOptionsText}
									loading={groupsLoading}
									disabled={groupsLoading}
									className="w-full"
								/>
							</>
						)}
					</div>
					<div className="space-y-1.5">
						<Label htmlFor={groupAmountId}>Spend limit ($)</Label>
						<Input
							id={groupAmountId}
							type="number"
							step="0.01"
							min="0.01"
							disabled={upsertPending}
							value={groupAmount}
							onChange={(event) => onGroupAmountChange(event.target.value)}
							placeholder="0.00"
						/>
					</div>
					{upsertError && (
						<p className="text-xs text-content-destructive">
							{getErrorMessage(upsertError, "Failed to save group override.")}
						</p>
					)}
				</div>

				<DialogFooter>
					<Button
						variant="outline"
						type="button"
						onClick={() => {
							onOpenChange(false);
							onSelectedGroupChange(null);
							onGroupAmountChange("");
						}}
						disabled={upsertPending}
					>
						Cancel
					</Button>
					<Button type="button" onClick={onSave} disabled={saveDisabled}>
						{upsertPending ? <Spinner loading className="h-4 w-4" /> : null}
						{isEditing ? "Update budget" : "Add group"}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
};
