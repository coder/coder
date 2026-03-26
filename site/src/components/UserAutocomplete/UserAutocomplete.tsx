import { useDebouncedFunction, useDebouncedValue } from "hooks/debounce";
import { type FC, useId, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { cn } from "utils/cn";
import { prepareQuery } from "utils/filters";
import { getErrorMessage } from "#/api/errors";
import { organizationMembers } from "#/api/queries/organizations";
import { users, workspaceAvailableUsers } from "#/api/queries/users";
import type {
	MinimalUser,
	OrganizationMemberWithUserData,
	User,
} from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Avatar } from "#/components/Avatar/Avatar";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { Button } from "#/components/Button/Button";
import {
	Combobox,
	ComboboxContent,
	ComboboxEmpty,
	ComboboxInput,
	ComboboxItem,
	ComboboxList,
	ComboboxTrigger,
} from "#/components/Combobox/Combobox";
import { Label } from "#/components/Label/Label";
import { Spinner } from "#/components/Spinner/Spinner";

// The common properties between users and org members that we need.
type SelectedUser = {
	avatar_url?: string;
	email?: string;
	username: string;
};

type CommonAutocompleteProps<T extends SelectedUser> = {
	className?: string;
	label?: string;
	onChange: (user: T | null) => void;
	value: T | null;
};

type UserAutocompleteProps = CommonAutocompleteProps<User>;

export const UserAutocomplete: FC<UserAutocompleteProps> = (props) => {
	const [filter, setFilter] = useState<string>();

	const usersQuery = useQuery({
		...users({
			q: prepareQuery(encodeURI(filter ?? "")),
			limit: 25,
		}),
		enabled: filter !== undefined,
		placeholderData: keepPreviousData,
	});
	return (
		<InnerAutocomplete<User>
			error={usersQuery.error}
			isFetching={usersQuery.isFetching}
			shouldFilter={false}
			setFilter={setFilter}
			users={usersQuery.data?.users}
			{...props}
		/>
	);
};

type MemberAutocompleteProps =
	CommonAutocompleteProps<OrganizationMemberWithUserData> & {
		organizationId: string;
	};

export const MemberAutocomplete: FC<MemberAutocompleteProps> = ({
	organizationId,
	...props
}) => {
	const [filter, setFilter] = useState<string>();

	const membersQuery = useQuery({
		...organizationMembers(organizationId),
		enabled: filter !== undefined,
		placeholderData: keepPreviousData,
	});
	return (
		<InnerAutocomplete<OrganizationMemberWithUserData>
			error={membersQuery.error}
			isFetching={membersQuery.isFetching}
			shouldFilter={true}
			setFilter={setFilter}
			users={membersQuery.data?.members}
			{...props}
		/>
	);
};

type WorkspaceUserAutocompleteProps = CommonAutocompleteProps<MinimalUser> & {
	organizationId: string;
};

export const WorkspaceUserAutocomplete: FC<WorkspaceUserAutocompleteProps> = ({
	organizationId,
	...props
}) => {
	const [filter, setFilter] = useState<string>();

	const availableUsersQuery = useQuery({
		...workspaceAvailableUsers(organizationId, {
			q: prepareQuery(encodeURI(filter ?? "")),
			limit: 25,
		}),
		enabled: filter !== undefined,
		placeholderData: keepPreviousData,
	});
	return (
		<InnerAutocomplete<MinimalUser>
			error={availableUsersQuery.error}
			isFetching={availableUsersQuery.isFetching}
			shouldFilter={false}
			setFilter={setFilter}
			users={availableUsersQuery.data}
			{...props}
		/>
	);
};

type InnerAutocompleteProps<T extends SelectedUser> =
	CommonAutocompleteProps<T> & {
		/** The error is null if not loaded or no error. */
		error: unknown;
		isFetching: boolean;
		/** Enable cmdk filtering for non-API-filtered variants. */
		shouldFilter: boolean;
		/** Filter is undefined if the autocomplete is closed. */
		setFilter: (filter: string | undefined) => void;
		/** Users are undefined if not loaded or errored. */
		users: readonly T[] | undefined;
	};

const InnerAutocomplete = <T extends SelectedUser>({
	className,
	error,
	label,
	onChange,
	shouldFilter,
	setFilter,
	isFetching,
	users,
	value,
}: InnerAutocompleteProps<T>) => {
	const DEBOUNCE_MS = 750;

	const [open, setOpen] = useState(false);
	const [inputValue, setInputValue] = useState("");
	const id = useId();
	const debouncedInputValue = useDebouncedValue(inputValue, DEBOUNCE_MS);
	const { debounced: debouncedSetFilter, cancelDebounce } =
		useDebouncedFunction((nextFilter: string) => {
			setFilter(nextFilter);
		}, DEBOUNCE_MS);

	const selectedInputValue = value?.email ?? value?.username ?? "";
	const selectedFilterValue = value?.username ?? "";
	// Keep spinner only while typing away from the selected value.
	const isLoadingOptions =
		selectedInputValue !== inputValue &&
		((inputValue !== "" && debouncedInputValue !== inputValue) || isFetching);

	const handleOpenChange = (nextOpen: boolean) => {
		setOpen(nextOpen);
		cancelDebounce();
		if (nextOpen) {
			setInputValue(selectedInputValue);
			setFilter(selectedFilterValue);
			return;
		}
		setInputValue("");
		setFilter(undefined);
	};

	return (
		<div className={cn("flex flex-col gap-2", className)}>
			{label && <Label htmlFor={id}>{label}</Label>}
			<Combobox
				value={value?.username}
				onValueChange={(newValue) => {
					if (!newValue) {
						onChange(null);
						return;
					}
					onChange(users?.find((user) => user.username === newValue) ?? null);
				}}
				open={open}
				onOpenChange={handleOpenChange}
			>
				<ComboboxTrigger asChild>
					<Button variant="outline">
						<span className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden">
							{value && (
								<Avatar
									size="sm"
									src={value.avatar_url}
									fallback={value.username}
								/>
							)}
							{value?.email ?? value?.username ?? "Select a user"}
						</span>
						<ChevronDownIcon className="p-0.5" />
					</Button>
				</ComboboxTrigger>
				<ComboboxContent
					className="w-[var(--radix-popover-trigger-width)] p-0"
					shouldFilter={shouldFilter}
				>
					<ComboboxInput
						placeholder="Username or email"
						value={inputValue}
						onValueChange={(nextInputValue) => {
							setInputValue(nextInputValue);
							if (nextInputValue === "") {
								// If we're clearing the input, we don't need
								// to debounce the filter change.
								cancelDebounce();
								setFilter("");
								return;
							}
							debouncedSetFilter(nextInputValue);
						}}
					/>
					<ComboboxList className="py-0.5">
						{!isLoadingOptions &&
							(users ?? []).map((option) => (
								<ComboboxItem
									key={option.username}
									value={option.username}
									keywords={[option.username, option.email ?? ""]}
									className="m-1"
								>
									<AvatarData
										title={option.username}
										subtitle={option.email}
										src={option.avatar_url}
									/>
								</ComboboxItem>
							))}
					</ComboboxList>
					<ComboboxEmpty>
						{isLoadingOptions || (!users && !error) ? (
							<div className="flex items-center justify-center py-2">
								<Spinner size="sm" loading />
							</div>
						) : error ? (
							getErrorMessage(error, "Unable to fetch users")
						) : (
							"No users found"
						)}
					</ComboboxEmpty>
				</ComboboxContent>
			</Combobox>
		</div>
	);
};
