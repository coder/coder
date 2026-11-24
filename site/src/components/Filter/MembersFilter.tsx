import { CircleDotIcon, TagIcon } from "lucide-react";
import {
	getValidationErrorMessage,
	hasError,
	isApiValidationError,
} from "#/api/errors";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";
import {
	getStatusFilterOptions,
	getUserTypeFilterOptions,
	USER_TYPE_CHIP_KEYS,
} from "#/components/Filter/userFilterOptions";

const categories: FilterCategory[] = [
	{
		key: "status",
		optionsKey: "members.status",
		label: "Status",
		icon: <CircleDotIcon />,
		inlineOptions: true,
		inlineOptionsIcons: true,
		getOptions: getStatusFilterOptions,
	},
	{
		key: "attribute",
		optionsKey: "members.attribute",
		label: "User type",
		icon: <TagIcon />,
		chipKeys: USER_TYPE_CHIP_KEYS,
		inlineOptions: true,
		inlineOptionsLabel: "User type is…",
		chipLabelOnly: true,
		getOptions: getUserTypeFilterOptions,
	},
];

type MembersFilterProps = {
	filter: UseFilterResult;
	error?: unknown;
};

/** Filters members by account status and type. */
export const MembersFilter: React.FC<MembersFilterProps> = ({
	filter,
	error,
}) => (
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
