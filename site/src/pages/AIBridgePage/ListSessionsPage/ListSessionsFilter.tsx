import type { FC } from "react";
import { DateTimeRangeFilter } from "#/components/DateTimeRangeFilter/DateTimeRangeFilter";
import type {
	FullTimeRange,
	TimeRange,
} from "#/components/DateTimeRangeFilter/timeRange";
import {
	Filter,
	MenuSkeleton,
	type useFilter,
} from "#/components/Filter/Filter";
import { type UserFilterMenu, UserMenu } from "#/components/Filter/UserFilter";
import { ClientFilter, type ClientFilterMenu } from "../filters/ClientFilter";
import { ModelFilter, type ModelFilterMenu } from "../filters/ModelFilter";
import {
	ProviderFilter,
	type ProviderFilterMenu,
} from "../filters/ProviderFilter";

// Narrower than the SelectFilter default so the search input keeps most
// of the row on wide viewports.
const FILTER_WIDTH = 150;

interface ListSessionsFilterProps {
	filter: ReturnType<typeof useFilter>;
	error?: unknown;
	menus: {
		user: UserFilterMenu;
		provider: ProviderFilterMenu;
		client: ClientFilterMenu;
		model: ModelFilterMenu;
	};
	timeRange: TimeRange;
	defaultTimeRange: FullTimeRange;
	onTimeRangeChange: (range: FullTimeRange) => void;
}

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	menus,
	timeRange,
	defaultTimeRange,
	onTimeRangeChange,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user.isInitializing}
			presets={[
				{
					name: "All sessions",
					query: "",
				},
				{
					name: "My sessions",
					query: "initiator:me",
				},
			]}
			error={error}
			options={
				<>
					<DateTimeRangeFilter
						value={timeRange}
						defaultValue={defaultTimeRange}
						onChange={onTimeRangeChange}
						width={FILTER_WIDTH}
					/>
					<UserMenu
						menu={menus.user}
						placeholder="All users"
						width={FILTER_WIDTH}
					/>
					<ProviderFilter menu={menus.provider} width={FILTER_WIDTH} />
					<ClientFilter menu={menus.client} width={FILTER_WIDTH} />
					<ModelFilter menu={menus.model} width={FILTER_WIDTH} />
				</>
			}
		/>
	);
};
