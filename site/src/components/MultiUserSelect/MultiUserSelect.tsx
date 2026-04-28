import { type FC, type ReactNode, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { organizationMembers } from "#/api/queries/organizations";
import { users } from "#/api/queries/users";
import type {
	OrganizationMemberWithUserData,
	ReducedUser,
	User,
} from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { AvatarDataSkeleton } from "#/components/Avatar/AvatarDataSkeleton";
import { Checkbox } from "#/components/Checkbox/Checkbox";
import { EmptyState } from "#/components/EmptyState/EmptyState";
import { SearchField } from "#/components/SearchField/SearchField";
import { useDebouncedFunction } from "#/hooks/debounce";
import { cn } from "#/utils/cn";
import { prepareQuery } from "#/utils/filters";

const DEBOUNCE_MS = 750;

type SelectedUser = ReducedUser | OrganizationMemberWithUserData;

type CommonMultiSelectProps<T extends SelectedUser> = {
	className?: string;
	onChange: (user: T, checked: boolean) => void;
	selected: T[];
	setFilter: (filter: string) => void;
};

type UserAutocompleteProps = CommonMultiSelectProps<User> & {
	filter: string;
};

export const MultiUserSelect: FC<UserAutocompleteProps> = ({
	filter,
	setFilter,
	...props
}) => {
	const usersQuery = useQuery({
		...users({
			q: prepareQuery(encodeURI(filter ?? "")),
			limit: 25,
		}),
		placeholderData: keepPreviousData,
	});
	return (
		<InnerMultiSelect<User>
			error={usersQuery.error}
			setFilter={setFilter}
			users={usersQuery.data?.users}
			{...props}
		/>
	);
};

type MemberAutocompleteProps =
	CommonMultiSelectProps<OrganizationMemberWithUserData> & {
		filter: string;
		organizationId: string;
	};

export const MultiMemberSelect: FC<MemberAutocompleteProps> = ({
	filter,
	organizationId,
	setFilter,
	...props
}) => {
	const membersQuery = useQuery({
		...organizationMembers(organizationId, {
			q: prepareQuery(encodeURI(filter ?? "")),
			limit: 25,
		}),
		placeholderData: keepPreviousData,
	});
	return (
		<InnerMultiSelect<OrganizationMemberWithUserData>
			error={membersQuery.error}
			setFilter={setFilter}
			users={membersQuery.data?.members}
			{...props}
		/>
	);
};

type InnerAutocompleteProps<T extends SelectedUser> =
	CommonMultiSelectProps<T> & {
		/** The error is null if not loaded or no error. */
		error: unknown;
		setFilter: (filter: string) => void;
		/** Users are undefined if not loaded or errored. */
		users: readonly T[] | undefined;
	};

const InnerMultiSelect = <T extends SelectedUser>({
	className,
	error,
	onChange,
	selected,
	setFilter,
	users,
}: InnerAutocompleteProps<T>) => {
	const [inputValue, setInputValue] = useState("");
	const { debounced, cancelDebounce } = useDebouncedFunction(
		(nextFilter: string) => {
			setFilter(nextFilter);
		},
		DEBOUNCE_MS,
	);

	return (
		<div className={cn("flex flex-col gap-4", className)}>
			<SearchField
				className="h-12 w-full rounded-lg"
				value={inputValue}
				aria-label="Search users"
				onChange={(query) => {
					setInputValue(query);
					debounced(query);
				}}
				onClear={() => {
					cancelDebounce();
					setInputValue("");
					setFilter("");
				}}
				placeholder="Search users..."
			/>
			<div className="h-96 w-full rounded-lg border border-border border-solid">
				<div className="h-full overflow-hidden p-px">
					<div
						className="h-full overflow-y-auto overflow-x-hidden overscroll-contain"
						onWheel={(event) => {
							event.stopPropagation();
						}}
					>
						<UsersTable
							error={error}
							onChange={onChange}
							selected={selected}
							users={users}
						/>
					</div>
				</div>
			</div>
		</div>
	);
};

type UsersTable<T extends SelectedUser> = {
	error: unknown;
	onChange: (user: T, checked: boolean) => void;
	selected: readonly T[];
	users: readonly T[] | undefined;
};

const UsersTable = <T extends SelectedUser>({
	error,
	onChange,
	selected,
	users,
}: UsersTable<T>) => {
	if (error) {
		return (
			<div className="p-3">
				<ErrorAlert error={error} />
			</div>
		);
	}

	if (!users) {
		return <TableLoader />;
	}

	if (users.length === 0) {
		return (
			<div className="p-3">
				<EmptyState message="No users found" isCompact />
			</div>
		);
	}

	return (
		<div className="flex flex-col">
			{users.map((user, index) => {
				const checked = selected.some((u) => userMatches(u, user));
				return (
					<UserRow
						key={user.username}
						user={user}
						checked={checked}
						isFirst={index === 0}
						isLast={index === users.length - 1}
						onChange={onChange}
					>
						<div className="flex items-center gap-3">
							<Checkbox
								data-testid={`checkbox-${user.username}`}
								checked={checked}
								onClick={(e) => {
									e.stopPropagation();
								}}
								onCheckedChange={(checked) => {
									onChange(user, Boolean(checked));
								}}
								aria-label={`Select user ${user.username}`}
							/>
							<AvatarData
								title={user.username}
								subtitle={user.email}
								src={user.avatar_url}
							/>
						</div>
					</UserRow>
				);
			})}
		</div>
	);
};

const TableLoader: FC = () => {
	const skeletonRows = Array.from({ length: 6 }, (_, index) => index);

	return (
		<div>
			{skeletonRows.map((row) => (
				<div className="flex min-h-[64px] items-center px-4 py-3" key={row}>
					<div className="flex items-center gap-3">
						<Checkbox disabled />
						<AvatarDataSkeleton />
					</div>
				</div>
			))}
		</div>
	);
};

interface UserRowProps<T extends SelectedUser> {
	checked: boolean;
	children?: ReactNode;
	isFirst: boolean;
	isLast: boolean;
	onChange: (user: T, checked: boolean) => void;
	user: T;
}

const UserRow = <T extends SelectedUser>({
	checked,
	children,
	isFirst,
	isLast,
	onChange,
	user,
}: UserRowProps<T>) => {
	return (
		<div
			data-testid={`user-${user.username}`}
			tabIndex={-1}
			className={cn(
				"cursor-pointer",
				"hover:[&>div]:ring-1 hover:[&>div]:ring-inset hover:[&>div]:ring-border-secondary",
				checked
					? "[&>div]:bg-surface-secondary hover:[&>div]:bg-surface-secondary"
					: undefined,
			)}
			onClick={() => onChange(user, !checked)}
			onKeyDown={(event) => {
				if (event.key === "Enter" || event.key === " ") {
					event.preventDefault();
					onChange(user, !checked);
				}
			}}
		>
			<div
				className={cn(
					"flex min-h-[64px] items-center px-4 py-3",
					isFirst && "rounded-tl-md rounded-tr-md",
					isLast && "rounded-bl-md rounded-br-md",
				)}
			>
				{children}
			</div>
		</div>
	);
};

function userMatches(a: SelectedUser, b: SelectedUser) {
	const aID = "user_id" in a ? a.user_id : a.id;
	const bID = "user_id" in b ? b.user_id : b.id;
	return aID && bID && aID === bID;
}
