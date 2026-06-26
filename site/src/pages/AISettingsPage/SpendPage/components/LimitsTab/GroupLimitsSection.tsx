import {
	CheckIcon,
	EllipsisVerticalIcon,
	PlusIcon,
	TrashIcon,
} from "lucide-react";
import { type FC, useId, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type { ChatUsageLimitGroupOverride, Group } from "#/api/typesGenerated";
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
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "#/components/DropdownMenu/DropdownMenu";
import { Input } from "#/components/Input/Input";
import { Label } from "#/components/Label/Label";
import { PaginationAmount } from "#/components/PaginationWidget/PaginationAmount";
import { PaginationWidgetBase } from "#/components/PaginationWidget/PaginationWidgetBase";
import { SearchField } from "#/components/SearchField/SearchField";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { getGroupSubtitle } from "#/modules/groups";
import {
	formatCostMicros,
	isPositiveFiniteDollarAmount,
} from "#/utils/currency";
import { paginateItems } from "#/utils/paginateItems";
import { ConfirmDeleteDialog } from "../ConfirmDeleteDialog";
import { SectionHeader } from "../SectionHeader";

const GROUP_LIMITS_PAGE_SIZE = 10;

interface GroupLimitsSectionProps {
	hideHeader?: boolean;
	groupOverrides: readonly ChatUsageLimitGroupOverride[];
	showGroupForm: boolean;
	onShowGroupFormChange: (show: boolean) => void;
	selectedGroup: Group | null;
	onSelectedGroupChange: (group: Group | null) => void;
	groupAmount: string;
	onGroupAmountChange: (amount: string) => void;
	availableGroups: Group[];
	groupAutocompleteNoOptionsText: string;
	groupsLoading: boolean;
	editingGroupOverride: {
		group_id: string;
		group_display_name: string;
		group_name: string;
		group_avatar_url: string;
		member_count: number;
	} | null;
	onEditGroupOverride: (
		override: GroupLimitsSectionProps["groupOverrides"][number],
	) => void;
	onAddGroupOverride: () => void;
	onDeleteGroupOverride: (groupID: string) => void;
	upsertPending: boolean;
	upsertError: Error | null;
	deletePending: boolean;
	deleteError: Error | null;
	groupsError: Error | null;
}

export const GroupLimitsSection: FC<GroupLimitsSectionProps> = ({
	hideHeader,
	groupOverrides,
	showGroupForm,
	onShowGroupFormChange,
	selectedGroup,
	onSelectedGroupChange,
	groupAmount,
	onGroupAmountChange,
	availableGroups,
	groupAutocompleteNoOptionsText,
	groupsLoading,
	editingGroupOverride,
	onEditGroupOverride,
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
	const isEditing = editingGroupOverride !== null;
	const [pendingDeleteGroupId, setPendingDeleteGroupId] = useState<
		string | null
	>(null);
	const [page, setPage] = useState(1);
	const [searchQuery, setSearchQuery] = useState("");
	const normalizedSearchQuery = searchQuery.trim().toLowerCase();
	const filteredGroupOverrides = normalizedSearchQuery
		? groupOverrides.filter((override) =>
				[override.group_display_name, override.group_name].some((value) =>
					value.toLowerCase().includes(normalizedSearchQuery),
				),
			)
		: groupOverrides;
	const { pagedItems, clampedPage, hasPreviousPage, hasNextPage } =
		paginateItems(filteredGroupOverrides, GROUP_LIMITS_PAGE_SIZE, page);

	return (
		<section className="space-y-6">
			{!hideHeader && (
				<SectionHeader
					label="Group limits"
					description="Override the default limit for specific groups. When a user belongs to multiple groups, the lowest group limit applies."
				/>
			)}
			<div className="space-y-6">
				<div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
					<SearchField
						value={searchQuery}
						onChange={(value) => {
							setSearchQuery(value);
							setPage(1);
						}}
						placeholder="Search groups..."
						aria-label="Search group limits"
						className="w-full sm:max-w-md"
					/>
					<Button
						variant="outline"
						size="lg"
						type="button"
						onClick={() => onShowGroupFormChange(true)}
						disabled={
							isEditing ||
							showGroupForm ||
							groupsLoading ||
							availableGroups.length === 0
						}
					>
						<PlusIcon />
						Add group
					</Button>
				</div>
				{filteredGroupOverrides.length > 0 ? (
					<div className="space-y-4">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>Group</TableHead>
									<TableHead>Members</TableHead>
									<TableHead>Spend limit</TableHead>
									<TableHead className="w-1">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{pagedItems.map((override) => (
									<TableRow key={override.group_id}>
										<TableCell>
											<AvatarData
												title={
													override.group_display_name || override.group_name
												}
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
										<TableCell className="w-1 whitespace-nowrap text-right">
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														variant="subtle"
														size="icon"
														type="button"
														disabled={deletePending || upsertPending}
														aria-label={`Actions for ${
															override.group_display_name || override.group_name
														}`}
													>
														<EllipsisVerticalIcon />
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem
														onSelect={() => onEditGroupOverride(override)}
													>
														Update budget
													</DropdownMenuItem>
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														disabled={isEditing}
														onSelect={() =>
															setPendingDeleteGroupId(override.group_id)
														}
													>
														<TrashIcon />
														Remove group limit
													</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						<PaginationAmount
							limit={GROUP_LIMITS_PAGE_SIZE}
							totalRecords={filteredGroupOverrides.length}
							currentOffsetStart={
								(clampedPage - 1) * GROUP_LIMITS_PAGE_SIZE + 1
							}
							paginationUnitLabel="groups"
							className="justify-end"
						/>
					</div>
				) : (
					<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
						{groupOverrides.length === 0
							? "No group overrides configured."
							: "No group limits match your search."}
					</div>
				)}

				{filteredGroupOverrides.length > GROUP_LIMITS_PAGE_SIZE && (
					<PaginationWidgetBase
						currentPage={clampedPage}
						pageSize={GROUP_LIMITS_PAGE_SIZE}
						totalRecords={filteredGroupOverrides.length}
						onPageChange={setPage}
						hasPreviousPage={hasPreviousPage}
						hasNextPage={hasNextPage}
					/>
				)}

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete group override.")}
					</p>
				)}

				<Dialog
					open={showGroupForm}
					onOpenChange={(open) => {
						if (upsertPending) {
							return;
						}

						onShowGroupFormChange(open);
						if (!open) {
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
											getOptionLabel={(group) =>
												group.display_name || group.name
											}
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
													{isSelected && (
														<CheckIcon className="size-4 shrink-0" />
													)}
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
									{getErrorMessage(
										upsertError,
										"Failed to save group override.",
									)}
								</p>
							)}
						</div>

						<DialogFooter>
							<Button
								variant="outline"
								type="button"
								onClick={() => {
									onShowGroupFormChange(false);
									onSelectedGroupChange(null);
									onGroupAmountChange("");
								}}
								disabled={upsertPending}
							>
								Cancel
							</Button>
							<Button
								type="button"
								onClick={() => void onAddGroupOverride()}
								disabled={
									isEditing
										? upsertPending ||
											!isPositiveFiniteDollarAmount(groupAmount)
										: upsertPending ||
											selectedGroup === null ||
											!isPositiveFiniteDollarAmount(groupAmount)
								}
							>
								{upsertPending ? <Spinner loading className="h-4 w-4" /> : null}
								{isEditing ? "Update budget" : "Add group"}
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
				{groupsError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(groupsError, "Failed to load groups.")}
					</p>
				)}
			</div>
			{pendingDeleteGroupId && (
				<ConfirmDeleteDialog
					entity="group override"
					onConfirm={() => {
						void onDeleteGroupOverride(pendingDeleteGroupId);
						setPendingDeleteGroupId(null);
					}}
					isPending={deletePending}
					open
					onOpenChange={(open) => !open && setPendingDeleteGroupId(null)}
				/>
			)}
		</section>
	);
};
