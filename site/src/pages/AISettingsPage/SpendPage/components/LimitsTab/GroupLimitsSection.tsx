import { PlusIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type { ChatUsageLimitGroupOverride, Group } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
import { SearchField } from "#/components/SearchField/SearchField";
import { paginateItems } from "#/utils/paginateItems";
import { SpendSectionHeader } from "../SpendSectionHeader";
import { GroupLimitDialog } from "./GroupLimitDialog";
import { GROUP_LIMITS_PAGE_SIZE, GroupLimitsTable } from "./GroupLimitsTable";

export interface GroupLimitOverrideGroup {
	group_id: string;
	group_display_name: string;
	group_name: string;
	group_avatar_url: string;
	member_count: number;
}

export type GroupLimitOverride = ChatUsageLimitGroupOverride;

interface GroupLimitsSectionProps {
	hideHeader?: boolean;
	groupOverrides: readonly GroupLimitOverride[];
	groupOrganizationNames?: Record<string, string | undefined>;
	showGroupForm: boolean;
	onShowGroupFormChange: (show: boolean) => void;
	selectedGroup: Group | null;
	onSelectedGroupChange: (group: Group | null) => void;
	groupAmount: string;
	onGroupAmountChange: (amount: string) => void;
	availableGroups: Group[];
	groupAutocompleteNoOptionsText: string;
	groupsLoading: boolean;
	editingGroupOverride: GroupLimitOverrideGroup | null;
	onEditGroupOverride: (override: GroupLimitOverride) => void;
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
	groupOrganizationNames,
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
				<SpendSectionHeader
					title="Group limits"
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

				<GroupLimitsTable
					pagedOverrides={pagedItems}
					totalOverrides={filteredGroupOverrides.length}
					clampedPage={clampedPage}
					hasPreviousPage={hasPreviousPage}
					hasNextPage={hasNextPage}
					onPageChange={setPage}
					onEditGroupOverride={onEditGroupOverride}
					onRequestDelete={setPendingDeleteGroupId}
					groupOrganizationNames={groupOrganizationNames}
					deletePending={deletePending}
					upsertPending={upsertPending}
					isEditing={isEditing}
					emptyMessage={
						groupOverrides.length === 0
							? "No group overrides configured."
							: "No group limits match your search."
					}
				/>

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete group override.")}
					</p>
				)}

				<GroupLimitDialog
					open={showGroupForm}
					onOpenChange={onShowGroupFormChange}
					selectedGroup={selectedGroup}
					onSelectedGroupChange={onSelectedGroupChange}
					groupAmount={groupAmount}
					onGroupAmountChange={onGroupAmountChange}
					availableGroups={availableGroups}
					groupAutocompleteNoOptionsText={groupAutocompleteNoOptionsText}
					groupsLoading={groupsLoading}
					editingGroupOverride={editingGroupOverride}
					upsertPending={upsertPending}
					upsertError={upsertError}
					onSave={onAddGroupOverride}
					groupAutocompleteId={groupAutocompleteId}
					groupAmountId={groupAmountId}
				/>
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
