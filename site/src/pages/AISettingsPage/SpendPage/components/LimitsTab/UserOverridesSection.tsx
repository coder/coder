import { PlusIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
import { getErrorMessage } from "#/api/errors";
import type { User } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
import { SearchField } from "#/components/SearchField/SearchField";
import { paginateItems } from "#/utils/paginateItems";
import { SpendSectionHeader } from "../SpendSectionHeader";
import { UserOverrideDialog } from "./UserOverrideDialog";
import {
	USER_OVERRIDES_PAGE_SIZE,
	UserOverridesTable,
} from "./UserOverridesTable";

export interface UserOverrideUser {
	user_id: string;
	name: string;
	username: string;
	avatar_url: string;
}

export interface UserOverride extends UserOverrideUser {
	spend_limit_micros: number | null;
}

interface UserOverridesSectionProps {
	hideHeader?: boolean;
	overrides: readonly UserOverride[];
	showUserForm: boolean;
	onShowUserFormChange: (show: boolean) => void;
	selectedUser: User | null;
	onSelectedUserChange: (user: User | null) => void;
	userOverrideAmount: string;
	onUserOverrideAmountChange: (amount: string) => void;
	selectedUserAlreadyOverridden: boolean;
	editingUserOverride: UserOverrideUser | null;
	onEditUserOverride: (override: UserOverride) => void;
	onAddOverride: () => void;
	onDeleteOverride: (userID: string) => void;
	upsertPending: boolean;
	upsertError: Error | null;
	deletePending: boolean;
	deleteError: Error | null;
}

export const UserOverridesSection: FC<UserOverridesSectionProps> = ({
	hideHeader,
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
	const [pendingDeleteUserId, setPendingDeleteUserId] = useState<string | null>(
		null,
	);
	const [page, setPage] = useState(1);
	const [searchQuery, setSearchQuery] = useState("");
	const normalizedSearchQuery = searchQuery.trim().toLowerCase();
	const filteredOverrides = normalizedSearchQuery
		? overrides.filter((override) =>
				[override.name, override.username].some((value) =>
					value.toLowerCase().includes(normalizedSearchQuery),
				),
			)
		: overrides;
	const { pagedItems, clampedPage, hasPreviousPage, hasNextPage } =
		paginateItems(filteredOverrides, USER_OVERRIDES_PAGE_SIZE, page);

	return (
		<section className="space-y-6">
			{!hideHeader && (
				<SpendSectionHeader
					title="Per-user overrides"
					description="Override the deployment default spend limit for specific users. User overrides take highest priority, followed by group limits, then the deployment default."
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
						placeholder="Search users..."
						aria-label="Search user overrides"
						className="w-full sm:max-w-md"
					/>
					<Button
						variant="outline"
						size="lg"
						type="button"
						onClick={() => onShowUserFormChange(true)}
						disabled={isEditing || showUserForm}
					>
						<PlusIcon />
						Add user
					</Button>
				</div>

				<UserOverridesTable
					pagedOverrides={pagedItems}
					totalOverrides={filteredOverrides.length}
					clampedPage={clampedPage}
					hasPreviousPage={hasPreviousPage}
					hasNextPage={hasNextPage}
					onPageChange={setPage}
					onEditUserOverride={onEditUserOverride}
					onRequestDelete={setPendingDeleteUserId}
					deletePending={deletePending}
					upsertPending={upsertPending}
					isEditing={isEditing}
					emptyMessage={
						overrides.length === 0
							? "No overrides configured."
							: "No user overrides match your search."
					}
				/>

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete override.")}
					</p>
				)}

				<UserOverrideDialog
					open={showUserForm}
					onOpenChange={onShowUserFormChange}
					selectedUser={selectedUser}
					onSelectedUserChange={onSelectedUserChange}
					userOverrideAmount={userOverrideAmount}
					onUserOverrideAmountChange={onUserOverrideAmountChange}
					selectedUserAlreadyOverridden={selectedUserAlreadyOverridden}
					editingUserOverride={editingUserOverride}
					upsertPending={upsertPending}
					upsertError={upsertError}
					onSave={onAddOverride}
					amountInputId={userOverrideAmountId}
				/>
			</div>
			{pendingDeleteUserId && (
				<ConfirmDeleteDialog
					entity="user override"
					onConfirm={() => {
						void onDeleteOverride(pendingDeleteUserId);
						setPendingDeleteUserId(null);
					}}
					isPending={deletePending}
					open
					onOpenChange={(open) => !open && setPendingDeleteUserId(null)}
				/>
			)}
		</section>
	);
};
