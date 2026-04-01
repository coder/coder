import { ChevronDown, UserPlusIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { getErrorMessage } from "#/api/errors";
import { users } from "#/api/queries/users";
import type { User } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import { Button } from "#/components/Button/Button";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { SearchField } from "#/components/SearchField/SearchField";
import { Spinner } from "#/components/Spinner/Spinner";
import { useDebouncedValue } from "#/hooks/debounce";
import { prepareQuery } from "#/utils/filters";

type AddUsersPopoverProps = {
	isLoading: boolean;
	onSubmit: (users: readonly User[]) => Promise<void>;
	onSuccess?: (users: readonly User[]) => void | Promise<void>;
	existingUserIds: ReadonlySet<string>;
};

export const AddUsersPopover: FC<AddUsersPopoverProps> = ({
	isLoading,
	onSubmit,
	onSuccess,
	existingUserIds,
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const debouncedSearch = useDebouncedValue(search, 400);
	const [selectedById, setSelectedById] = useState(
		() => new Map<string, User>(),
	);

	const usersQuery = useQuery({
		...users({
			q: prepareQuery(debouncedSearch),
			limit: 50,
		}),
		enabled: open,
		placeholderData: keepPreviousData,
	});

	const addableUsers =
		usersQuery.data?.users.filter((u) => !existingUserIds.has(u.id)) ?? [];

	const resetPanel = () => {
		setSearch("");
		setSelectedById(new Map());
	};

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		if (!nextOpen) {
			resetPanel();
		}
	};

	const toggleUser = (user: User) => {
		setSelectedById((prev) => {
			const next = new Map(prev);
			if (next.has(user.id)) {
				next.delete(user.id);
			} else {
				next.set(user.id, user);
			}
			return next;
		});
	};

	const selectedCount = selectedById.size;
	const selectedList = Array.from(selectedById.values());

	const handleAddUsers = async () => {
		if (selectedCount === 0) {
			return;
		}

		try {
			await onSubmit(selectedList);
			await onSuccess?.(selectedList);
			resetPanel();
			setOpen(false);
		} catch {
			// Parent is expected to show a toast. Keep panel open for retry.
		}
	};

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild>
				<Button variant="outline" className="shrink-0">
					<UserPlusIcon className="size-icon-sm" />
					Add users
					<ChevronDown className="size-icon-sm opacity-70" aria-hidden />
				</Button>
			</PopoverTrigger>
			{open && (
				<PopoverContent
					align="end"
					className="w-[min(100vw-2rem,32rem)] max-w-none p-0 flex flex-col gap-0"
				>
					<div className="flex flex-col gap-2 p-4">
						<SearchField
							value={search}
							onChange={setSearch}
							placeholder="Search users…"
							aria-label="Search users to add"
							autoFocus
						/>
						<div
							className="max-h-64 overflow-y-auto rounded-md border border-solid"
							data-testid="add-members-user-list"
						>
							{usersQuery.isFetching && !usersQuery.data ? (
								<div className="flex justify-center py-8">
									<Spinner size="sm" loading />
								</div>
							) : usersQuery.error ? (
								<p className="px-3 py-4 text-sm text-content-destructive">
									{getErrorMessage(usersQuery.error, "Unable to load users.")}
								</p>
							) : addableUsers.length === 0 ? (
								<EmptyState
									message={
										debouncedSearch.trim() === ""
											? "No users available to add."
											: "No users match your search."
									}
									isCompact
								/>
							) : (
								<ul
									className="divide-y divide-border list-none m-0 p-0"
									aria-label="Users available to add"
								>
									{addableUsers.map((user) => {
										const checked = selectedById.has(user.id);
										const checkboxId = `add-member-${user.id}`;
										return (
											<li key={user.id} className="pl-0">
												<label
													htmlFor={checkboxId}
													className="flex cursor-pointer items-center gap-3 px-2 py-2 hover:bg-surface-secondary"
												>
													<Checkbox
														id={checkboxId}
														checked={checked}
														disabled={isLoading}
														onCheckedChange={() => {
															toggleUser(user);
														}}
													/>
													<Avatar
														size="sm"
														src={user.avatar_url}
														fallback={user.username}
													/>
													<div className="min-w-0 flex-1">
														<div className="truncate font-medium text-content-primary text-sm">
															{user.name || user.username}
														</div>
														<div className="truncate text-content-secondary text-xs">
															{user.email || "—"}
														</div>
													</div>
												</label>
											</li>
										);
									})}
								</ul>
							)}
						</div>
					</div>
					<div className="flex flex-row justify-end gap-2 border-0 border-t border-border border-solid px-4 py-3">
						<Button
							type="button"
							variant="outline"
							onClick={() => {
								handleOpenChange(false);
							}}
						>
							Cancel
						</Button>
						<Button
							type="button"
							disabled={selectedCount === 0 || isLoading}
							onClick={() => {
								void handleAddUsers();
							}}
						>
							<Spinner loading={isLoading}>
								<UserPlusIcon className="size-icon-sm" />
							</Spinner>
							Add users
						</Button>
					</div>
				</PopoverContent>
			)}
		</Popover>
	);
};
