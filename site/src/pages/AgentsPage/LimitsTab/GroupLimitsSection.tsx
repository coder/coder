import { getErrorMessage } from "api/errors";
import type { Group } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { AvatarData } from "components/Avatar/AvatarData";
import { Button } from "components/Button/Button";
import { Input } from "components/Input/Input";
import { Label } from "components/Label/Label";
import { Spinner } from "components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "components/Table/Table";
import { Check } from "lucide-react";
import { getGroupSubtitle } from "modules/groups";
import { type FC, useId } from "react";
import { formatCostMicros, isPositiveFiniteDollarAmount } from "utils/currency";
import { SectionHeader } from "../SectionHeader";

interface GroupLimitsSectionProps {
	groupOverrides: ReadonlyArray<{
		group_id: string;
		group_display_name: string;
		group_name: string;
		group_avatar_url: string;
		member_count: number;
		spend_limit_micros: number | null;
	}>;
	panelClassName: string;
	showGroupForm: boolean;
	onShowGroupFormChange: (show: boolean) => void;
	selectedGroup: Group | null;
	onSelectedGroupChange: (group: Group | null) => void;
	groupAmount: string;
	onGroupAmountChange: (amount: string) => void;
	availableGroups: Group[];
	groupAutocompleteNoOptionsText: string;
	groupsLoading: boolean;
	onAddGroupOverride: () => void;
	onDeleteGroupOverride: (groupID: string) => void;
	upsertPending: boolean;
	upsertError: Error | null;
	deletePending: boolean;
	deleteError: Error | null;
	groupsError: Error | null;
}

export const GroupLimitsSection: FC<GroupLimitsSectionProps> = ({
	groupOverrides,
	panelClassName,
	showGroupForm,
	onShowGroupFormChange,
	selectedGroup,
	onSelectedGroupChange,
	groupAmount,
	onGroupAmountChange,
	availableGroups,
	groupAutocompleteNoOptionsText,
	groupsLoading,
	onAddGroupOverride,
	onDeleteGroupOverride,
	upsertPending,
	upsertError,
	deletePending,
	deleteError,
	groupsError,
}) => {
	const groupAutocompleteId = useId();
	const groupAmountId = useId();

	return (
		<section className="space-y-4">
			<SectionHeader
				label="Group Limits"
				description="Override the default limit for specific groups. When a user belongs to multiple groups, the lowest group limit applies."
			/>

			<div className={panelClassName}>
				{groupOverrides.length > 0 ? (
					<Table>
						<TableHeader>
							<TableRow>
								<TableHead>Group</TableHead>
								<TableHead>Members</TableHead>
								<TableHead>Spend Limit</TableHead>
								<TableHead className="w-[80px]">Actions</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{groupOverrides.map((override) => (
								<TableRow key={override.group_id}>
									<TableCell>
										<AvatarData
											title={override.group_display_name || override.group_name}
											subtitle={override.group_name}
											src={override.group_avatar_url}
											imgFallbackText={override.group_name}
										/>
									</TableCell>
									<TableCell>{override.member_count}</TableCell>
									<TableCell>
										{override.spend_limit_micros !== null
											? formatCostMicros(override.spend_limit_micros)
											: "Unlimited"}
									</TableCell>
									<TableCell>
										<Button
											variant="outline"
											size="sm"
											type="button"
											onClick={() =>
												void onDeleteGroupOverride(override.group_id)
											}
											disabled={deletePending}
										>
											Delete
										</Button>
									</TableCell>
								</TableRow>
							))}
						</TableBody>
					</Table>
				) : (
					<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
						No group overrides configured.
					</div>
				)}

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete group override.")}
					</p>
				)}

				{!showGroupForm ? (
					<Button
						variant="outline"
						size="sm"
						type="button"
						onClick={() => onShowGroupFormChange(true)}
						disabled={groupsLoading || availableGroups.length === 0}
					>
						Add Group
					</Button>
				) : (
					<div className="space-y-3 rounded-lg border border-border bg-surface-secondary/40 p-4">
						<div className="flex flex-col gap-3 md:flex-row md:items-end">
							<div className="flex-1 space-y-1">
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
											{isSelected && <Check className="size-4 shrink-0" />}
										</div>
									)}
									placeholder="Search groups..."
									noOptionsText={groupAutocompleteNoOptionsText}
									loading={groupsLoading}
									disabled={groupsLoading}
									className="w-full"
								/>
							</div>
							<div className="flex-1 space-y-1">
								<Label htmlFor={groupAmountId}>Spend Limit ($)</Label>
								<Input
									id={groupAmountId}
									type="number"
									step="0.01"
									min="0"
									className="h-9 min-w-0 text-[13px]"
									value={groupAmount}
									onChange={(event) => onGroupAmountChange(event.target.value)}
									placeholder="0.00"
								/>
							</div>
							<div className="flex gap-2 md:pb-0.5">
								<Button
									size="sm"
									type="button"
									onClick={() => void onAddGroupOverride()}
									disabled={
										upsertPending ||
										selectedGroup === null ||
										!isPositiveFiniteDollarAmount(groupAmount)
									}
								>
									{upsertPending ? (
										<Spinner loading className="h-4 w-4" />
									) : null}
									Add
								</Button>
								<Button
									variant="outline"
									size="sm"
									type="button"
									onClick={() => {
										onShowGroupFormChange(false);
										onSelectedGroupChange(null);
										onGroupAmountChange("");
									}}
								>
									Cancel
								</Button>
							</div>
						</div>
					</div>
				)}
				{upsertError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(upsertError, "Failed to save group override.")}
					</p>
				)}
				{groupsError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(groupsError, "Failed to load groups.")}
					</p>
				)}
			</div>
		</section>
	);
};
