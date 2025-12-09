import Autocomplete from "@mui/material/Autocomplete";
import CircularProgress from "@mui/material/CircularProgress";
import TextField from "@mui/material/TextField";
import { templaceACLAvailable } from "api/queries/templates";
import type { Group, ReducedUser } from "api/typesGenerated";
import {
	autocompleteClassName,
	isGroup,
	UserOrGroupOption,
} from "components/UserOrGroupAutocomplete/UserOrGroupOption";
import { useDebouncedFunction } from "hooks/debounce";
import { type ChangeEvent, type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { prepareQuery } from "utils/filters";

export type UserOrGroupAutocompleteValue = ReducedUser | Group | null;
type AutocompleteOption = Exclude<UserOrGroupAutocompleteValue, null>;

type UserOrGroupAutocompleteProps = {
	value: UserOrGroupAutocompleteValue;
	onChange: (value: UserOrGroupAutocompleteValue) => void;
	templateID: string;
	exclude: UserOrGroupAutocompleteValue[];
};

export const UserOrGroupAutocomplete: FC<UserOrGroupAutocompleteProps> = ({
	value,
	onChange,
	templateID,
	exclude,
}) => {
	const [autoComplete, setAutoComplete] = useState({
		value: "",
		open: false,
	});
	const aclAvailableQuery = useQuery({
		...templaceACLAvailable(templateID, {
			q: prepareQuery(encodeURI(autoComplete.value)),
			limit: 25,
		}),
		enabled: autoComplete.open,
		placeholderData: keepPreviousData,
	});
	const options: AutocompleteOption[] = aclAvailableQuery.data
		? [
				...aclAvailableQuery.data.groups,
				...aclAvailableQuery.data.users,
			].filter((result) => {
				const excludeIds = exclude.map(
					(optionToExclude) => optionToExclude?.id,
				);
				return !excludeIds.includes(result.id);
			})
		: [];

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
			value={value}
			id="user-or-group-autocomplete"
			open={autoComplete.open}
			onOpen={() => {
				setAutoComplete((state) => ({
					...state,
					open: true,
				}));
			}}
			onClose={() => {
				setAutoComplete({
					value: isGroup(value) ? value.display_name : (value?.email ?? ""),
					open: false,
				});
			}}
			onChange={(_, newValue) => {
				onChange(newValue);
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
			loading={aclAvailableQuery.isFetching}
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
								{aclAvailableQuery.isFetching ? (
									<CircularProgress size={16} />
								) : null}
								{params.InputProps.endAdornment}
							</>
						),
					}}
				/>
			)}
		/>
	);
};
