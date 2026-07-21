import { CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
import { keepPreviousData, useQuery } from "react-query";
import { groupsByOrganization } from "#/api/queries/groups";
import { organizationMembers } from "#/api/queries/organizations";
import type {
	Group,
	OrganizationMemberWithUserData,
} from "#/api/typesGenerated";
import { Autocomplete } from "#/components/Autocomplete/Autocomplete";
import { AvatarData } from "#/components/Avatar/AvatarData";
import { getGroupSubtitle, isGroup } from "#/modules/groups";

type OrganizationMember = OrganizationMemberWithUserData & { id: string };
type ResourceSharingCandidate = OrganizationMember | Group;
export type ResourceSharingCandidateValue = ResourceSharingCandidate | null;

type ResourceSharingAutocompleteProps = {
	value: ResourceSharingCandidateValue;
	onChange: (value: ResourceSharingCandidateValue) => void;
	organizationId: string;
	excludeIds: readonly string[];
};

const normalizeMember = (
	member: OrganizationMemberWithUserData,
): OrganizationMember => ({
	...member,
	id: member.user_id,
});

export const UserOrGroupAutocomplete: FC<ResourceSharingAutocompleteProps> = ({
	value,
	onChange,
	organizationId,
	excludeIds,
}) => {
	const [inputValue, setInputValue] = useState("");
	const [open, setOpen] = useState(false);

	const membersQuery = useQuery({
		...organizationMembers(organizationId, { limit: 0 }),
		enabled: open,
		placeholderData: keepPreviousData,
	});
	const groupsQuery = useQuery({
		...groupsByOrganization(organizationId),
		enabled: open,
		placeholderData: keepPreviousData,
	});

	const filterValue = inputValue.trim().toLowerCase();
	const groups = (groupsQuery.data ?? []).filter((group) => {
		if (!filterValue) {
			return true;
		}
		return `${group.display_name ?? ""} ${group.name}`
			.toLowerCase()
			.includes(filterValue);
	});
	const users = (membersQuery.data?.members ?? [])
		.filter((member) => {
			if (!filterValue) {
				return true;
			}
			return `${member.name ?? ""} ${member.username} ${member.email}`
				.toLowerCase()
				.includes(filterValue);
		})
		.map(normalizeMember);
	const options = [...groups, ...users].filter(
		(option) => !excludeIds.includes(option.id),
	);

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
				<div className="flex w-full items-center justify-between">
					<AvatarData
						title={
							isGroup(option)
								? option.display_name || option.name
								: option.username
						}
						subtitle={isGroup(option) ? getGroupSubtitle(option) : option.email}
						src={option.avatar_url}
					/>
					{isSelected && <CheckIcon className="size-4 shrink-0" />}
				</div>
			)}
			open={open}
			onOpenChange={(nextOpen) => {
				setOpen(nextOpen);
				if (!nextOpen) {
					setInputValue("");
				}
			}}
			inputValue={inputValue}
			onInputChange={setInputValue}
			loading={membersQuery.isFetching || groupsQuery.isFetching}
			placeholder="Search for user or group"
			noOptionsText="No users or groups found"
			className="w-full sm:w-80"
			id="ai-resource-user-or-group-autocomplete"
		/>
	);
};
