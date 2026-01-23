import { groupsByOrganization } from "api/queries/groups";
import { paginatedOrganizationMembers } from "api/queries/organizations";
import type { Group, OrganizationMemberWithUserData } from "api/typesGenerated";
import { Autocomplete } from "components/Autocomplete/Autocomplete";
import { AvatarData } from "components/Avatar/AvatarData";
import { usePaginatedQuery } from "hooks/usePaginatedQuery";
import { Check } from "lucide-react";
import { getGroupSubtitle, isGroup } from "modules/groups";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";

type OrganizationMember = OrganizationMemberWithUserData & { id: string };
type AutocompleteOption = OrganizationMember | Group;
export type UserOrGroupAutocompleteValue = AutocompleteOption | null;

type ExcludableOption = { id?: string | null } | null;

type UserOrGroupAutocompleteProps = {
	value: UserOrGroupAutocompleteValue;
	onChange: (value: UserOrGroupAutocompleteValue) => void;
	organizationId: string;
	exclude: ExcludableOption[];
};

const normalizeMember = (
	member: OrganizationMemberWithUserData,
): OrganizationMember => ({
	...member,
	id: member.user_id,
});

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

	// Use org members endpoint instead of site-wide /users endpoint.
	// This allows regular org members to see other members in their org
	// for workspace sharing, without needing site-wide user:read permission.
	const membersQuery = usePaginatedQuery({
		...paginatedOrganizationMembers(organizationId, new URLSearchParams()),
		enabled: open,
		placeholderData: keepPreviousData,
		prefetch: false,
	});

	const groupsQuery = useQuery({
		...groupsByOrganization(organizationId),
		enabled: open,
		placeholderData: keepPreviousData,
	});

	const filterValue = inputValue.trim().toLowerCase();

	// Filter groups by search input (client-side filtering).
	const groupOptions = groupsQuery.data
		? groupsQuery.data.filter((group) => {
				if (!filterValue) {
					return true;
				}
				const haystack = `${group.display_name ?? ""} ${group.name}`.trim();
				return haystack.toLowerCase().includes(filterValue);
			})
		: [];

	// Filter members by search input (client-side filtering since org members
	// endpoint doesn't support search params).
	const userOptions = membersQuery.data?.members
		? membersQuery.data.members
				.filter((member) => {
					if (!filterValue) {
						return true;
					}
					const haystack =
						`${member.name ?? ""} ${member.username} ${member.email}`.toLowerCase();
					return haystack.includes(filterValue);
				})
				.map(normalizeMember)
		: [];

	const excludeIds = exclude
		.map((optionToExclude) => optionToExclude?.id)
		.filter((id): id is string => Boolean(id));

	const options: AutocompleteOption[] = [
		...groupOptions,
		...userOptions,
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
			loading={membersQuery.isFetching || groupsQuery.isFetching}
			placeholder="Search for user or group"
			noOptionsText="No users or groups found"
			className="w-80"
			id="workspace-user-or-group-autocomplete"
		/>
	);
};
