import { CircleDotIcon, TagIcon } from "lucide-react";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import { userStatusOptions } from "#/components/Filter/UsersFilter";

const categories: FilterCategory[] = [
	{
		key: "status",
		optionsKey: "members.status",
		label: "Status",
		icon: <CircleDotIcon />,
		inlineOptions: true,
		inlineOptionsIcons: true,
		getOptions: async () => userStatusOptions,
	},
	{
		key: "attribute",
		optionsKey: "members.attribute",
		label: "Attributes",
		icon: <TagIcon />,
		chipKeys: ["service_account"],
		inlineOptions: true,
		inlineOptionsLabel: "Member is…",
		chipLabelOnly: true,
		getOptions: async () => [
			{
				value: "service_account",
				label: "Service account",
				token: "service_account:true",
			},
		],
	},
];

type OrganizationMembersFilterProps = {
	filter: UseFilterResult;
	error?: unknown;
};

/** Filters organization members by account status and type. */
export const OrganizationMembersFilter: React.FC<
	OrganizationMembersFilterProps
> = ({ filter, error }) => (
	<FilterCombobox
		value={filter.query}
		onChange={filter.update}
		categories={categories}
		placeholder="Search and filter members…"
		className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
		errorMessage={
			hasError(error) && isApiValidationError(error)
				? getValidationErrorMessage(error)
				: undefined
		}
	/>
);
