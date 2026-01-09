import { templaceACLAvailable } from "api/queries/templates";
import type { Group, ReducedUser } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { AvatarData } from "components/Avatar/AvatarData";
import { Check } from "lucide-react";
import { getGroupSubtitle, isGroup } from "modules/groups";
import { type FC, useState } from "react";
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
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);

	const handleOpenChange = (newOpen: boolean) => {
		setOpen(newOpen);
		if (!newOpen) {
			setInputValue("");
		}
	};

	const aclAvailableQuery = useQuery({
		...templaceACLAvailable(templateID, {
			q: prepareQuery(encodeURI(inputValue)),
			limit: 25,
		}),
		enabled: open,
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

	return (
		<Autocomplete
			value={value}
			onChange={onChange}
			options={options}
			getOptionValue={(option) => option.id}
			getOptionLabel={(option) =>
				isGroup(option) ? option.display_name || option.name : option.email
			}
			isOptionEqualToValue={(option, optionValue) =>
				option.id === optionValue.id
			}
			renderOption={(option, isSelected) => (
				<div className="flex items-center justify-between w-full">
					<AvatarData
						title={
							isGroup(option)
								? option.display_name || option.name
								: option.username
						}
						subtitle={isGroup(option) ? getGroupSubtitle(option) : option.email}
						src={option.avatar_url}
					/>
					{isSelected && <Check className="size-4 shrink-0" />}
				</div>
			)}
			open={open}
			onOpenChange={handleOpenChange}
			inputValue={inputValue}
			onInputChange={setInputValue}
			loading={aclAvailableQuery.isFetching}
			placeholder="Search for user or group"
			noOptionsText="No users or groups found"
			className="w-[300px]"
			id="user-or-group-autocomplete"
		/>
	);
};
