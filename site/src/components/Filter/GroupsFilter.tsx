import type { FC } from "react";
import { Filter, type useFilter } from "#/components/Filter/Filter";

interface GroupsFilterProps {
	filter: ReturnType<typeof useFilter>;
}

// GroupsFilter renders a search-only filter. Groups support free-text search
// against name and display name, so there are no presets or option menus.
export const GroupsFilter: FC<GroupsFilterProps> = ({ filter }) => {
	return (
		<Filter
			presets={[]}
			isLoading={false}
			filter={filter}
			optionsSkeleton={null}
		/>
	);
};
