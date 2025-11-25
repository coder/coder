import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { groupsByOrganization } from "api/queries/groups";
import { users } from "api/queries/users";
import type { Group, User } from "api/typesGenerated";
import {
	autocompleteClassName,
	isGroup,
	UserOrGroupOption,
} from "components/UserOrGroupAutocomplete/UserOrGroupOption";
import { useDebouncedFunction } from "hooks/debounce";
import { type ChangeEvent, type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { prepareQuery } from "utils/filters";

type AutocompleteOption = User | Group;
export type UserOrGroupAutocompleteValue = AutocompleteOption | null;

type ExcludableOption = { id?: string | null } | null;

type UserOrGroupAutocompleteProps = {
	value: UserOrGroupAutocompleteValue;
	onChange: (value: UserOrGroupAutocompleteValue) => void;
	organizationId: string;
	exclude: ExcludableOption[];
};

export const UserOrGroupAutocomplete: FC<UserOrGroupAutocompleteProps> = ({
	value,
	onChange,
	organizationId,
	exclude,
}) => {
	const [autoComplete, setAutoComplete] = useState({
		value: "",
		open: false,
	});

	const usersQuery = useQuery({
		...users({
			q: prepareQuery(encodeURI(autoComplete.value)),
			limit: 25,
		}),
		enabled: autoComplete.open,
		placeholderData: keepPreviousData,
	});

	const groupsQuery = useQuery({
		...groupsByOrganization(organizationId),
		enabled: autoComplete.open,
		placeholderData: keepPreviousData,
	});

	const filterValue = autoComplete.value.trim().toLowerCase();
	const groupOptions = groupsQuery.data
		? groupsQuery.data.filter((group) => {
				if (!filterValue) {
					return true;
				}
				const haystack = `${group.display_name ?? ""} ${group.name}`.trim();
				return haystack.toLowerCase().includes(filterValue);
			})
		: [];

	const excludeIds = exclude
		.map((optionToExclude) => optionToExclude?.id)
		.filter((id): id is string => Boolean(id));

	const options: AutocompleteOption[] = [
		...groupOptions,
		...(usersQuery.data?.users ?? []),
	].filter((result) => !excludeIds.includes(result.id));

	const { debounced: handleFilterChange } = useDebouncedFunction(
		(event: ChangeEvent<HTMLInputElement>) => {
			setAutoComplete((state) => ({
				...state,
				value: event.target.value,
			}));
		},
		500,
	);

	return (
		<Autocomplete<AutocompleteOption, false, false, false>
			noOptionsText="No users or groups found"
			value={value ?? null}
			id="workspace-user-or-group-autocomplete"
			open={autoComplete.open}
			onOpen={() => {
				setAutoComplete((state) => ({
					...state,
					open: true,
				}));
			}}
			onClose={() => {
				setAutoComplete({
					value: isGroup(value)
						? value.display_name || value.name
						: (value?.email ?? value?.username ?? ""),
					open: false,
				});
			}}
			onChange={(_, newValue) => {
				onChange(newValue ?? null);
			}}
			isOptionEqualToValue={(option, optionValue) =>
				optionValue !== null && option.id === optionValue.id
			}
			getOptionLabel={(option) =>
				isGroup(option) ? option.display_name || option.name : option.email
			}
			renderOption={(props, option) => (
				<UserOrGroupOption htmlProps={props} option={option} />
			)}
			options={options}
			loading={usersQuery.isFetching || groupsQuery.isFetching}
			className={autocompleteClassName}
			renderInput={(params) => (
				<TextField
					{...params}
					margin="none"
					size="small"
					placeholder="Search for user or group"
					InputProps={{
						...params.InputProps,
						onChange: handleFilterChange,
						endAdornment: (
							<>
								{(usersQuery.isFetching || groupsQuery.isFetching) && (
									<CircularProgress size={16} />
								)}
								{params.InputProps.endAdornment}
							</>
						),
					}}
				/>
			)}
		/>
	);
};
