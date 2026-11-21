import type { UseFilterResult } from "#/components/Filter/Filter";
import { FilterCombobox } from "#/components/Filter/FilterCombobox/FilterCombobox";
import type { FilterCategory } from "#/components/Filter/FilterCombobox/types";

const categories: FilterCategory[] = [];

type GroupsFilterProps = {
	filter: UseFilterResult;
};

/** Searches group names and display names using plain text. */
export const GroupsFilter: React.FC<GroupsFilterProps> = ({ filter }) => (
	<FilterCombobox
		value={filter.query}
		onChange={filter.update}
		categories={categories}
		placeholder="Search groups…"
		className="w-full min-w-0 self-start sm:w-auto sm:min-w-lg sm:max-w-full"
	/>
);
