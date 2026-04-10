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
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from "#/components/Table/Table";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import { useDebouncedFunction } from "#/hooks/debounce";
import { useClickableTableRow } from "#/hooks/useClickableTableRow";
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
		<div className={cn("flex flex-col gap-2", className)}>
			<SearchField
				className="w-full"
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
			<div className="max-h-[360px] overflow-auto">
				<Table>
					<TableBody className="[&_td]:h-[72px]">
						<UsersTable
							error={error}
							onChange={onChange}
							selected={selected}
							users={users}
						/>
					</TableBody>
				</Table>
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
			<TableRow>
				<TableCell colSpan={999}>
					<ErrorAlert error={error} />
				</TableCell>
			</TableRow>
		);
	}

	if (!users) {
		return <TableLoader />;
	}

	if (users.length === 0) {
		return (
			<TableRow>
				<TableCell colSpan={999}>
					<EmptyState message="No users found" isCompact />
				</TableCell>
			</TableRow>
		);
	}

	return users.map((user) => {
		const checked = selected.some((u) => userMatches(u, user));
		return (
			<UserRow
				key={user.username}
				user={user}
				checked={checked}
				onChange={onChange}
			>
				<TableCell className="border-0">
					<div className="flex items-center gap-5">
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
				</TableCell>
			</UserRow>
		);
	});
};

const TableLoader: FC = () => {
	return (
		<TableLoaderSkeleton>
			<TableRowSkeleton>
				<TableCell className="w-2/6">
					<div className="flex items-center gap-5">
						<Checkbox disabled />
						<AvatarDataSkeleton />
					</div>
				</TableCell>
			</TableRowSkeleton>
		</TableLoaderSkeleton>
	);
};

interface UserRowProps<T extends SelectedUser> {
	checked: boolean;
	children?: ReactNode;
	onChange: (user: T, checked: boolean) => void;
	user: T;
}

const UserRow = <T extends SelectedUser>({
	checked,
	children,
	onChange,
	user,
}: UserRowProps<T>) => {
	const clickableProps = useClickableTableRow({
		onClick: () => onChange(user, !checked),
	});
	return (
		<TableRow
			{...clickableProps}
			data-testid={`user-${user.username}`}
			className={cn([
				checked ? "bg-muted hover:bg-muted" : undefined,
				clickableProps.className,
			])}
		>
			{children}
		</TableRow>
	);
};

function userMatches(a: SelectedUser, b: SelectedUser) {
	const aID = "user_id" in a ? a.user_id : a.id;
	const bID = "user_id" in b ? b.user_id : b.id;
	return aID && bID && aID === bID;
}
