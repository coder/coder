import { EllipsisVerticalIcon, PlusIcon, TrashIcon } from "lucide-react";
import { type FC, useId, useState } from "react";
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
import { ConfirmDeleteDialog } from "#/components/Dialogs/ConfirmDeleteDialog/ConfirmDeleteDialog";
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
import { UserAutocomplete } from "#/components/UserAutocomplete/UserAutocomplete";
import {
	formatCostMicros,
	isPositiveFiniteDollarAmount,
} from "#/utils/currency";
import { paginateItems } from "#/utils/paginateItems";
import { SectionHeader } from "../SectionHeader";

const USER_OVERRIDES_PAGE_SIZE = 10;

interface UserOverridesSectionProps {
	hideHeader?: boolean;
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
				<SectionHeader
					label="Per-user overrides"
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
				{filteredOverrides.length > 0 ? (
					<div className="space-y-4">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>User</TableHead>
									<TableHead>Spend limit</TableHead>
									<TableHead className="w-1">Actions</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{pagedItems.map((override) => (
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
										<TableCell className="w-1 whitespace-nowrap text-right">
											<DropdownMenu>
												<DropdownMenuTrigger asChild>
													<Button
														variant="subtle"
														size="icon"
														type="button"
														disabled={deletePending || upsertPending}
														aria-label={`Actions for ${override.name || override.username}`}
													>
														<EllipsisVerticalIcon />
													</Button>
												</DropdownMenuTrigger>
												<DropdownMenuContent align="end">
													<DropdownMenuItem
														onSelect={() => onEditUserOverride(override)}
													>
														Update budget
													</DropdownMenuItem>
													<DropdownMenuItem
														className="text-content-destructive focus:text-content-destructive"
														disabled={isEditing}
														onSelect={() =>
															setPendingDeleteUserId(override.user_id)
														}
													>
														<TrashIcon />
														Remove override
													</DropdownMenuItem>
												</DropdownMenuContent>
											</DropdownMenu>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
						<PaginationAmount
							limit={USER_OVERRIDES_PAGE_SIZE}
							totalRecords={filteredOverrides.length}
							currentOffsetStart={
								(clampedPage - 1) * USER_OVERRIDES_PAGE_SIZE + 1
							}
							paginationUnitLabel="users"
							className="justify-end"
						/>
					</div>
				) : (
					<div className="rounded-lg border border-border bg-surface-secondary px-4 py-6 text-center text-sm text-content-secondary">
						{overrides.length === 0
							? "No overrides configured."
							: "No user overrides match your search."}
					</div>
				)}

				{filteredOverrides.length > USER_OVERRIDES_PAGE_SIZE && (
					<PaginationWidgetBase
						currentPage={clampedPage}
						pageSize={USER_OVERRIDES_PAGE_SIZE}
						totalRecords={filteredOverrides.length}
						onPageChange={setPage}
						hasPreviousPage={hasPreviousPage}
						hasNextPage={hasNextPage}
					/>
				)}

				{deleteError && (
					<p className="text-xs text-content-destructive">
						{getErrorMessage(deleteError, "Failed to delete override.")}
					</p>
				)}

				<Dialog
					open={showUserForm}
					onOpenChange={(open) => {
						if (upsertPending) {
							return;
						}

						onShowUserFormChange(open);
						if (!open) {
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
							<div className="space-y-1.5">
								<Label htmlFor={userOverrideAmountId}>Spend limit ($)</Label>
								<Input
									id={userOverrideAmountId}
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
									onShowUserFormChange(false);
									onSelectedUserChange(null);
									onUserOverrideAmountChange("");
								}}
								disabled={upsertPending}
							>
								Cancel
							</Button>
							<Button
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
								{upsertPending ? <Spinner loading className="h-4 w-4" /> : null}
								{isEditing ? "Update budget" : "Add user"}
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
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
