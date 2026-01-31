import { getErrorMessage } from "api/errors";
import { organizationMembers } from "api/queries/organizations";
import { users } from "api/queries/users";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { useDebouncedFunction } from "hooks/debounce";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { prepareQuery } from "utils/filters";

// The common properties between users and org members that we need.
type SelectedUser = {
	avatar_url?: string;
	email: string;
	username: string;
};

type CommonAutocompleteProps<T extends SelectedUser> = {
	className?: string;
	label?: string;
	onChange: (user: T | null) => void;
	required?: boolean;
	value: T | null;
};

type UserAutocompleteProps = CommonAutocompleteProps<User>;

export const UserAutocomplete: FC<UserAutocompleteProps> = (props) => {
	const [filter, setFilter] = useState<string | undefined>("");

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
			setFilter={setFilter}
			users={membersQuery.data?.members}
			{...props}
		/>
	);
};

type InnerAutocompleteProps<T extends SelectedUser> =
	CommonAutocompleteProps<T> & {
		/** The error is null if not loaded or no error. */
		error: unknown;
		/** Filter is undefined if the autocomplete is closed. */
		setFilter: (filter: string | undefined) => void;
		/** Users are undefined if not loaded or errored. */
		users: readonly T[] | undefined;
		required?: boolean;
	};

const InnerAutocomplete = <T extends SelectedUser>({
	className,
	error,
	label,
	onChange,
	setFilter,
	users,
	value,
	required,
}: InnerAutocompleteProps<T>) => {
	const [inputValue, setInputValue] = useState("");

	const { debounced: debouncedInputOnChange } = useDebouncedFunction(
		(value: string) => {
			setFilter(value ?? "");
		},
		750,
	);

	return (
		<Autocomplete
			value={value}
			onChange={onChange}
			options={users ?? []}
			getOptionValue={(option) => option.username}
			getOptionLabel={(option) => option.email}
			isOptionEqualToValue={(a, b) => a.username === b.username}
			renderOption={(option) => (
				<AvatarData
					title={option.username}
					subtitle={option.email}
					src={option.avatar_url}
				/>
			)}
			label={label}
			placeholder="User email or username"
			noOptionsText={
				error
					? getErrorMessage(error, "Unable to fetch users")
					: "No users found"
			}
			loading={!users && !error}
			inputValue={inputValue}
			onInputChange={(newValue) => {
				setInputValue(newValue);
				debouncedInputOnChange(newValue);
			}}
			onOpenChange={(isOpen) => {
				if (isOpen) {
					setFilter("");
				} else {
					setFilter(undefined);
				}
			}}
			required={required}
			startAdornment={
				value && (
					<Avatar size="sm" src={value.avatar_url} fallback={value.username} />
				)
			}
			className={className}
			data-testid="user-autocomplete"
		/>
	);
};
