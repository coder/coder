import { groupsByOrganization } from "api/queries/groups";
import { users } from "api/queries/users";
import type { Group, User } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { AvatarData } from "components/Avatar/AvatarData";
import { isGroup } from "components/UserOrGroupAutocomplete/UserOrGroupOption";
import { Check } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { prepareQuery } from "utils/filters";
import { getGroupSubtitle } from "utils/groups";

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
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);

	const handleOpenChange = (newOpen: boolean) => {
		setOpen(newOpen);
		if (!newOpen) {
			setInputValue("");
		}
	};

	const usersQuery = useQuery({
		...users({
			q: prepareQuery(encodeURI(inputValue)),
			limit: 25,
		}),
		enabled: open,
		placeholderData: keepPreviousData,
	});

	const groupsQuery = useQuery({
		...groupsByOrganization(organizationId),
		enabled: open,
		placeholderData: keepPreviousData,
	});

	const filterValue = inputValue.trim().toLowerCase();
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
			loading={usersQuery.isFetching || groupsQuery.isFetching}
			placeholder="Search for user or group"
			noOptionsText="No users or groups found"
			className="w-80"
			id="workspace-user-or-group-autocomplete"
		/>
	);
};
