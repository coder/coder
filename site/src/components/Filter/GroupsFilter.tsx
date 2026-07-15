import type { FC } from "react";
import { Filter, type useFilter } from "#/components/Filter/Filter";

interface GroupsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
}

// GroupsFilter renders a search-only filter. Groups support free-text search
// against name and display name, so there are no presets or option menus.
export const GroupsFilter: FC<GroupsFilterProps> = ({ filter, error }) => {
	return (
		<Filter
			presets={[]}
			isLoading={false}
			filter={filter}
			error={error}
			optionsSkeleton={null}
		/>
	);
};
