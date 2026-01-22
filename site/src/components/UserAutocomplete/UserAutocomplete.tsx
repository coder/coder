import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { getErrorMessage } from "api/errors";
import { organizationMembers } from "api/queries/organizations";
import { users } from "api/queries/users";
import type { OrganizationMemberWithUserData, User } from "api/typesGenerated";
import { Avatar } from "components/Avatar/Avatar";
import { AvatarData } from "components/Avatar/AvatarData";
import { useDebouncedFunction } from "hooks/debounce";
import {
	type ChangeEvent,
	type ComponentProps,
	type FC,
	useState,
} from "react";
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
	size?: ComponentProps<typeof TextField>["size"];
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
		isFetching: boolean;
		/** Filter is undefined if the autocomplete is closed. */
		setFilter: (filter: string | undefined) => void;
		/** Users are undefined if not loaded or errored. */
		users: readonly T[] | undefined;
	};

const InnerAutocomplete = <T extends SelectedUser>({
	className,
	error,
	isFetching,
	label,
	onChange,
	required,
	setFilter,
	size = "small",
	users,
	value,
}: InnerAutocompleteProps<T>) => {
	const [open, setOpen] = useState(false);

	const { debounced: debouncedInputOnChange } = useDebouncedFunction(
		(event: ChangeEvent<HTMLInputElement>) => {
			setFilter(event.target.value ?? "");
		},
		750,
	);

	return (
		<Autocomplete
			noOptionsText={
				error
					? getErrorMessage(error, "Unable to fetch users")
					: "No users found"
			}
			className={className}
			options={users ?? []}
			loading={!users && !error}
			value={value}
			data-testid="user-autocomplete"
			open={open}
			isOptionEqualToValue={(a, b) => a.username === b.username}
			getOptionLabel={(option) => option.email}
			onOpen={() => {
				setOpen(true);
				setFilter(value?.email ?? "");
			}}
			onClose={() => {
				setOpen(false);
				setFilter(undefined);
			}}
			onChange={(_, newValue) => {
				onChange(newValue);
			}}
			renderOption={({ key, ...props }, option) => (
				<li key={key} {...props}>
					<AvatarData
						title={option.username}
						subtitle={option.email}
						src={option.avatar_url}
					/>
				</li>
			)}
			renderInput={(params) => (
				<TextField
					{...params}
					required={required}
					fullWidth
					size={size}
					label={label}
					placeholder="User email or username"
					css={{
						"&:not(:has(label))": {
							margin: 0,
						},
					}}
					InputProps={{
						...params.InputProps,
						onChange: debouncedInputOnChange,
						startAdornment: value && (
							<Avatar
								size="sm"
								src={value.avatar_url}
								fallback={value.username}
							/>
						),
						endAdornment: (
							<>
								{isFetching && open && <CircularProgress size={16} />}
								{params.InputProps.endAdornment}
							</>
						),
						classes: { root: "!pl-3.5 gap-1" },
					}}
					InputLabelProps={{
						shrink: true,
					}}
				/>
			)}
		/>
	);
};
