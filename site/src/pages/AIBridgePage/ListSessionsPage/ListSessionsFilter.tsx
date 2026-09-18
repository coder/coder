import type { FC } from "react";
import { DateTimeRangePicker } from "#/components/DateTimeRangePicker/DateTimeRangePicker";
import type { DateTimeRangeValue } from "#/components/DateTimeRangePicker/dateTimeRange";
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
import { formatTimeRangeQuery, parseTimeRangeQuery } from "./timeRange";

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
	timeRange: DateTimeRangeValue;
	onTimeRangeChange: (value: DateTimeRangeValue) => void;
}

export const ListSessionsFilter: FC<ListSessionsFilterProps> = ({
	filter,
	error,
	menus,
	timeRange,
	onTimeRangeChange,
}) => {
	return (
		<Filter
			filter={filter}
			optionsSkeleton={<MenuSkeleton />}
			isLoading={menus.user.isInitializing}
			// No preset queries; the search field and menus already cover them.
			presets={[]}
			error={error}
			formatQuery={formatTimeRangeQuery}
			parseQuery={parseTimeRangeQuery}
			options={
				<>
					<DateTimeRangePicker
						value={timeRange}
						onChange={onTimeRangeChange}
						size="lg"
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
